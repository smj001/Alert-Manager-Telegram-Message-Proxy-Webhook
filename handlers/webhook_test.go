package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/services"
)

func TestHealthMethod(t *testing.T) {
	w := httptest.NewRecorder()
	(&WebhookHandler{}).HandleHealth(w, httptest.NewRequest(http.MethodPost, "/health", nil))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("health POST: status=%d allow=%q", w.Code, w.Header().Get("Allow"))
	}
}

type recorder struct {
	calls int
	jobs  []models.WebhookRequest
	err   error
}

func (q *recorder) Enqueue(batch []models.WebhookRequest) error {
	q.calls++
	q.jobs = batch
	return q.err
}

func TestWebhookAdmission(t *testing.T) {
	const valid = `{"chat_id":-42,"message":"hello"}`
	for _, tt := range []struct {
		name, method, auth, body string
		queueErr                 error
		status, calls            int
	}{
		{"unauthorized", "POST", "", valid, nil, 401, 0},
		{"method", "GET", "Bearer test", valid, nil, 405, 0},
		{"accepted", "POST", "bearer test", valid, nil, 202, 1},
		{"invalid json", "POST", "Bearer test", `{`, nil, 400, 0},
		{"trailing value", "POST", "Bearer test", valid + ` {}`, nil, 400, 0},
		{"body too large", "POST", "Bearer test", valid + strings.Repeat(" ", maxBodyBytes), nil, 413, 0},
		{"invalid chat", "POST", "Bearer test", `{"message":"hello"}`, nil, 400, 0},
		{"empty message", "POST", "Bearer test", `{"chat_id":1,"message":"  "}`, nil, 400, 0},
		{"wrong media type", "POST", "Bearer test", `{"chat_id":1,"media_url":"https://example.com/p","media_type":"unknown"}`, nil, 400, 0},
		{"missing media URL", "POST", "Bearer test", `{"chat_id":1,"message":"hello","media_type":"photo"}`, nil, 400, 0},
		{"invalid media URL", "POST", "Bearer test", `{"chat_id":1,"media_url":"/image.jpg","media_type":"photo"}`, nil, 400, 0},
		{"media only", "POST", "Bearer test", `{"chat_id":1,"media_url":"https://example.com/p","media_type":"photo"}`, nil, 202, 1},
		{"Unicode", "POST", "Bearer test", `{"chat_id":1,"message":"` + strings.Repeat("界", 4096) + `"}`, nil, 202, 1},
		{"text too long", "POST", "Bearer test", `{"chat_id":1,"message":"` + strings.Repeat("x", 4097) + `"}`, nil, 400, 0},
		{"backpressure", "POST", "Bearer test", valid, services.ErrQueueUnavailable, 503, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &recorder{err: tt.queueErr}
			w := webhookRequest(NewWebhookHandler(q, "test").HandleWebhook, tt.method, tt.auth, tt.body)
			if w.Code != tt.status || q.calls != tt.calls {
				t.Fatalf("status=%d calls=%d body=%s, want %d/%d", w.Code, q.calls, w.Body, tt.status, tt.calls)
			}
			if w.Code == 202 && (w.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(w.Body.String()) != `{"status":"queued"}`) {
				t.Fatalf("unexpected acceptance response: %v %s", w.Header(), w.Body)
			}
		})
	}
}

func TestAlertRoutingAndAtomicValidation(t *testing.T) {
	const first = `{"status":"firing","labels":{"alertname":"HighCPU","chat_id":"-13"}}`
	for _, tt := range []struct {
		name, body string
		status     int
		chats      []int64
	}{
		{"mixed recipients", `{"alerts":[` + first + `,{"status":"resolved","labels":{"chat_id":"14"}}]}`, 202, []int64{-13, 14}},
		{"common fallback", `{"commonLabels":{"chat_id":"15"},"alerts":[{"status":"resolved"}]}`, 202, []int64{15}},
		{"invalid second recipient", `{"alerts":[` + first + `,{"labels":{"chat_id":"0"}}]}`, 400, nil},
		{"missing second recipient", `{"alerts":[` + first + `,{}]}`, 400, nil},
		{"no alerts", `{"alerts":[]}`, 400, nil},
		{"too many alerts", `{"alerts":[` + strings.Repeat(first+`,`, models.MaxAlertBatch) + first + `]}`, 400, nil},
		{"not an array", `{"alerts":{}}`, 400, nil},
		{"long alert", `{"commonLabels":{"chat_id":"15"},"alerts":[{"status":"resolved","annotations":{"description":"` + strings.Repeat("界", 5000) + `"}}]}`, 202, []int64{15}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &recorder{}
			w := webhookRequest(NewAlertManagerHandler(q, "test").HandleAlertManagerWebhook, "POST", "Bearer test", tt.body)
			if w.Code != tt.status || len(q.jobs) != len(tt.chats) {
				t.Fatalf("status=%d calls=%d jobs=%d body=%s", w.Code, q.calls, len(q.jobs), w.Body)
			}
			if tt.status != 202 {
				if q.calls != 0 {
					t.Fatal("invalid batch reached queue")
				}
				return
			}
			if q.calls != 1 {
				t.Fatalf("expected one batch, got %d", q.calls)
			}
			for i, chat := range tt.chats {
				status := "RESOLVED"
				if chat == -13 {
					status = "FIRING"
				}
				if q.jobs[i].ChatID != chat || !strings.Contains(q.jobs[i].Message, status) || len([]rune(q.jobs[i].Message)) > 4096 {
					t.Fatalf("incorrect alert status, chat, or length for %d", chat)
				}
			}
		})
	}
}

func webhookRequest(handler http.HandlerFunc, method, auth, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set("Authorization", auth)
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestAlertDigestIsolatedByChat(t *testing.T) {
	const body = `{"commonLabels":{"chat_id":"1"},"alerts":[
		{"status":"firing","labels":{"alertname":"CPUHigh","severity":"critical","instance":"worker-1"},"annotations":{"summary":"<load>","description":"CPU usage crossed 90%."},"startsAt":"2026-11-19T10:27:00+02:00","generatorURL":"https://prom.example/graph"},
		{"status":"resolved","labels":{"chat_id":"2","alertname":"SecretOtherChat"}},
		{"status":"resolved","labels":{"alertname":"MemoryHigh"},"endsAt":"2026-11-19T09:30:00Z","generatorURL":"javascript:example"}]}`
	q := &recorder{}
	w := webhookRequest(NewAlertManagerHandler(q, "test").HandleAlertManagerWebhook, "POST", "Bearer test", body)
	if w.Code != 202 || q.calls != 1 || len(q.jobs) != 2 {
		t.Fatalf("status=%d enqueue calls=%d messages=%d", w.Code, q.calls, len(q.jobs))
	}
	first, second := q.jobs[0], q.jobs[1]
	if first.ChatID != 1 || second.ChatID != 2 || strings.Contains(first.Message, "SecretOtherChat") || strings.Contains(second.Message, "CPUHigh") {
		t.Fatalf("chat isolation failure: %+v", q.jobs)
	}
	for _, part := range []string{"2 alerts", "1 firing", "1 resolved", "1. 🔴 FIRING · CPUHigh", "2. ✅ RESOLVED · MemoryHigh", "CRITICAL", "worker-1", "<load>", "CPU usage crossed 90%.", "08:27 UTC", "09:30 UTC"} {
		if !strings.Contains(first.Message, part) {
			t.Errorf("digest missing %q:\n%s", part, first.Message)
		}
	}
	if strings.Contains(first.Message, "prom.example") || strings.Contains(first.Message, "javascript:") || strings.Contains(first.Message, "Page 1/1") {
		t.Fatalf("unwanted content in digest:\n%s", first.Message)
	}
}

func TestAlertDigestSingleAlertLayout(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
	}{
		{
			"firing with multiline runbook",
			`{"status":"firing","labels":{"alertname":"BackupStale","severity":"critical","instance":"gitlab.example"},"startsAt":"2026-11-19T03:31:00Z","annotations":{"summary":"No verified backup for >26h","description":"1. Check /var/log/backup.log\r\n   grep 'disk  full' /var/log/backup.log\r\n2. Confirm at least 40 GB free.\r\n\r\n\r\nIf needed, check https://docs.example/backup."},"generatorURL":"http://prometheus.monitoring:9090/graph?g0.expr=backup_query"}`,
			"📟 Alertmanager\n\n🔴 FIRING · BackupStale\nSeverity: CRITICAL · Instance: gitlab.example\nSince: 2026-11-19 03:31 UTC\n\nNo verified backup for >26h\n\nDetails\n1. Check /var/log/backup.log\n   grep 'disk  full' /var/log/backup.log\n2. Confirm at least 40 GB free.\n\nIf needed, check https://docs.example/backup.",
		},
		{
			"resolved without duplicate details",
			`{"status":"resolved","labels":{"alertname":"BackupStale"},"endsAt":"2026-11-19T05:31:00+02:00","annotations":{"summary":"Backup verified","description":" Backup\nverified "},"generatorURL":"https://prom.example/graph"}`,
			"📟 Alertmanager\n\n✅ RESOLVED · BackupStale\nCleared: 2026-11-19 03:31 UTC\n\nBackup verified",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &recorder{}
			body := `{"commonLabels":{"chat_id":"1"},"alerts":[` + tt.body + `]}`
			w := webhookRequest(NewAlertManagerHandler(q, "test").HandleAlertManagerWebhook, "POST", "Bearer test", body)
			if w.Code != 202 || len(q.jobs) != 1 {
				t.Fatalf("status=%d messages=%d", w.Code, len(q.jobs))
			}
			if got := q.jobs[0].Message; got != tt.want {
				t.Fatalf("message:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestAlertDigestKeepsLongDuplicateTextOnce(t *testing.T) {
	text := strings.Repeat("backups ", 60) + "verify checksum"
	page := digestPages([]models.AlertManagerAlert{{
		Status:      "firing",
		Annotations: map[string]string{"summary": text, "description": text},
	}})[0]
	if strings.Count(page, text) != 1 {
		t.Fatalf("shared summary and description should appear in full once:\n%s", page)
	}
}

func TestAlertDigestPaginatesMaximumBatch(t *testing.T) {
	for _, tt := range []struct {
		name, detail string
		count        int
	}{
		{"many short alerts", "short", models.MaxAlertBatch},
		{"long emoji alerts", strings.Repeat("🚨", 1000), 15},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := models.AlertManagerWebhook{CommonLabels: map[string]string{"chat_id": "1"}}
			for i := 1; i <= tt.count; i++ {
				payload.Alerts = append(payload.Alerts, models.AlertManagerAlert{
					Status: "firing", Labels: map[string]string{"alertname": fmt.Sprintf("ALERT-%04d", i)},
					Annotations: map[string]string{"description": tt.detail},
				})
			}
			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			q := &recorder{}
			w := webhookRequest(NewAlertManagerHandler(q, "test").HandleAlertManagerWebhook, "POST", "Bearer test", string(body))
			if w.Code != 202 || q.calls != 1 || len(q.jobs) < 2 || len(q.jobs) >= tt.count {
				t.Fatalf("status=%d calls=%d pages=%d alerts=%d", w.Code, q.calls, len(q.jobs), tt.count)
			}
			var digest strings.Builder
			for i, page := range q.jobs {
				if page.ChatID != 1 || !page.SuppressLinkPreview || len(utf16.Encode([]rune(page.Message))) > models.MaxMessageSize || !strings.Contains(page.Message, fmt.Sprintf("Page %d/%d", i+1, len(q.jobs))) {
					t.Fatalf("incorrect chat, size, or numbering on page %d", i+1)
				}
				if cards := strings.Count(page.Message, ". 🔴 FIRING · ALERT-"); cards > 10 || (tt.name == "many short alerts" && cards != 10) {
					t.Fatalf("unexpected number of cards on page %d: %d", i+1, cards)
				}
				digest.WriteString(page.Message)
			}
			text := digest.String()
			for i := 1; i <= tt.count; i++ {
				name := fmt.Sprintf("ALERT-%04d", i)
				pos := strings.Index(text, name)
				if pos < 0 || strings.Count(text, name) != 1 {
					t.Fatalf("alert %s missing, duplicated, or out of order", name)
				}
				text = text[pos+len(name):]
			}
		})
	}
}
