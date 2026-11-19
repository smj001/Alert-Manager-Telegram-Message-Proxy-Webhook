package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
)

const (
	alertsPerPage = 10
	cardSeparator = "\n\n"
	sourceLabel   = "Open in Prometheus"
	sourcePrefix  = "\n\n🔗 "
	maxSourceURL  = 4096
)

type digestPage struct {
	text     string
	entities []models.MessageEntity
}

func digestPages(alerts []models.AlertManagerAlert) []digestPage {
	header := "📟 Alertmanager"
	if len(alerts) > 1 {
		firing := 0
		for _, alert := range alerts {
			if alert.Status == "firing" {
				firing++
			}
		}
		header += fmt.Sprintf("\n%d alerts · 🔴 %d firing · ✅ %d resolved", len(alerts), firing, len(alerts)-firing)
	}
	// Worst-case page-number width is the alert count: one card per page.
	reserved := telegramTextLength(fmt.Sprintf("%s\nPage %d/%d%s", header, len(alerts), len(alerts), cardSeparator))
	var pages []digestPage
	var page strings.Builder
	var entities []models.MessageEntity
	size, cards := 0, 0
	for i, alert := range alerts {
		number := i + 1
		if len(alerts) == 1 {
			number = 0
		}
		card := renderAlert(number, alert)
		length := telegramTextLength(card)
		source := alertSource(alert.GeneratorURL)
		if source != "" {
			length += telegramTextLength(sourcePrefix) + len(sourceLabel)
		}
		if cards != 0 && (cards == alertsPerPage || reserved+size+len(cardSeparator)+length > models.MaxMessageSize) {
			pages = append(pages, digestPage{text: page.String(), entities: entities})
			page.Reset()
			entities = nil
			size, cards = 0, 0
		}
		if cards != 0 {
			page.WriteString(cardSeparator)
			size += len(cardSeparator)
		}
		page.WriteString(card)
		if source != "" {
			page.WriteString(sourcePrefix)
			page.WriteString(sourceLabel)
			entities = append(entities, models.MessageEntity{
				Type: "text_link", Offset: size + length - len(sourceLabel), Length: len(sourceLabel), URL: source,
			})
		}
		size += length
		cards++
	}
	if cards != 0 {
		pages = append(pages, digestPage{text: page.String(), entities: entities})
	}
	for i := range pages {
		prefix := header
		if len(pages) > 1 {
			prefix += fmt.Sprintf("\nPage %d/%d", i+1, len(pages))
		}
		prefix += cardSeparator
		pages[i].text = prefix + pages[i].text
		headerLength := telegramTextLength(prefix)
		for j := range pages[i].entities {
			pages[i].entities[j].Offset += headerLength
		}
	}
	return pages
}

func alertSource(raw string) string {
	if raw == "" || len(raw) > maxSourceURL || strings.ContainsAny(raw, "\\<>\"") || strings.IndexFunc(raw, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	if strings.HasSuffix(u.Host, ":") {
		return ""
	}
	if port := u.Port(); port != "" {
		if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
			return ""
		}
	}
	if _, err := url.QueryUnescape(u.RawQuery); err != nil {
		return ""
	}
	return raw
}

func renderAlert(number int, alert models.AlertManagerAlert) string {
	status, timeLabel, rawTime := "🔴 FIRING", "Since", alert.StartsAt
	if alert.Status == "resolved" {
		status, timeLabel, rawTime = "✅ RESOLVED", "Cleared", alert.EndsAt
	}
	name := truncateAlertText(oneLine(alert.Labels["alertname"]), 160)
	if name == "" {
		name = "Unnamed alert"
	}
	var card strings.Builder
	if number > 0 {
		fmt.Fprintf(&card, "%d. ", number)
	}
	fmt.Fprintf(&card, "%s · %s", status, name)
	var metadata []string
	if severity := truncateAlertText(strings.ToUpper(oneLine(alert.Labels["severity"])), 40); severity != "" {
		metadata = append(metadata, "Severity: "+severity)
	}
	if instance := truncateAlertText(oneLine(alert.Labels["instance"]), 160); instance != "" {
		metadata = append(metadata, "Instance: "+instance)
	}
	if len(metadata) > 0 {
		card.WriteByte('\n')
		card.WriteString(strings.Join(metadata, " · "))
	}
	if timestamp, err := time.Parse(time.RFC3339Nano, rawTime); err == nil && !timestamp.IsZero() {
		fmt.Fprintf(&card, "\n%s: %s", timeLabel, timestamp.UTC().Format("2006-01-02 15:04 UTC"))
	}
	summary := oneLine(alert.Annotations["summary"])
	description := alert.Annotations["description"]
	normalizedDescription := oneLine(description)
	if summary != "" {
		card.WriteString(cardSeparator)
		if summary == normalizedDescription && telegramTextLength(summary) > 400 {
			// Keep the details budget when the only annotation text is shared.
			card.WriteString(truncateAlertText(multilineAlertText(description), 1300))
		} else {
			card.WriteString(truncateAlertText(summary, 400))
		}
	}
	if normalizedDescription != "" && normalizedDescription != summary {
		card.WriteString(cardSeparator + "Details\n")
		card.WriteString(truncateAlertText(multilineAlertText(description), 1300))
	}
	return card.String()
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func multilineAlertText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var lines strings.Builder
	blank := false
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			blank = true
			continue
		}
		if lines.Len() > 0 {
			lines.WriteByte('\n')
			if blank {
				lines.WriteByte('\n')
			}
		}
		lines.WriteString(line)
		blank = false
	}
	return lines.String()
}

// Telegram describes the limit in characters; budgeting UTF-16 units is
// conservative for emoji and avoids splitting a UTF-8 code point.
func telegramTextLength(s string) int {
	size := 0
	for _, r := range s {
		size += telegramRuneLength(r)
	}
	return size
}

func telegramRuneLength(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	return 1
}

func truncateAlertText(text string, limit int) string {
	size, cut := 0, 0
	for i, r := range text {
		size += telegramRuneLength(r)
		if size > limit {
			return strings.TrimSpace(text[:cut]) + "…"
		}
		if size < limit {
			cut = i + utf8.RuneLen(r)
		}
	}
	return text
}
