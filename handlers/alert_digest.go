package handlers

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
)

const (
	alertsPerPage = 10
	cardSeparator = "\n\n"
)

func digestPages(alerts []models.AlertManagerAlert) []string {
	const title = "📟 Alertmanager"
	if len(alerts) == 1 {
		return []string{title + cardSeparator + renderAlert(0, alerts[0])}
	}
	firing := 0
	for _, alert := range alerts {
		if alert.Status == "firing" {
			firing++
		}
	}
	header := fmt.Sprintf("%s\n%d alerts · 🔴 %d firing · ✅ %d resolved", title, len(alerts), firing, len(alerts)-firing)
	// Worst-case page-number width is the alert count: one card per page.
	reserved := telegramTextLength(fmt.Sprintf("%s\nPage %d/%d%s", header, len(alerts), len(alerts), cardSeparator))
	var pages []string
	var page strings.Builder
	size, cards := 0, 0
	for i, alert := range alerts {
		card := renderAlert(i+1, alert)
		length := telegramTextLength(card)
		if cards != 0 && (cards == alertsPerPage || reserved+size+len(cardSeparator)+length > models.MaxMessageSize) {
			pages = append(pages, page.String())
			page.Reset()
			size, cards = 0, 0
		}
		if cards != 0 {
			page.WriteString(cardSeparator)
			size += len(cardSeparator)
		}
		page.WriteString(card)
		size += length
		cards++
	}
	pages = append(pages, page.String())
	for i, text := range pages {
		if len(pages) > 1 {
			pages[i] = fmt.Sprintf("%s\nPage %d/%d%s%s", header, i+1, len(pages), cardSeparator, text)
		} else {
			pages[i] = header + cardSeparator + text
		}
	}
	return pages
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
