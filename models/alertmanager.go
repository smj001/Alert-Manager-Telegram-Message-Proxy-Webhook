package models

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type AlertManagerAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     string            `json:"startsAt"`
	EndsAt       string            `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

type AlertManagerWebhook struct {
	Version           string             `json:"version"`
	GroupKey          string             `json:"groupKey"`
	TruncatedAlerts   int                `json:"truncatedAlerts"`
	Status            string             `json:"status"`
	Receiver          string             `json:"receiver"`
	GroupLabels       map[string]string  `json:"groupLabels"`
	CommonLabels      map[string]string  `json:"commonLabels"`
	CommonAnnotations map[string]string  `json:"commonAnnotations"`
	ExternalURL       string             `json:"externalURL"`
	Alerts            AlertManagerAlerts `json:"alerts"`
}

type AlertManagerAlerts []AlertManagerAlert

func (a *AlertManagerAlerts) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('[') {
		return fmt.Errorf("alerts must be a JSON array")
	}
	var alerts AlertManagerAlerts
	for decoder.More() {
		if len(alerts) == MaxAlertBatch {
			return fmt.Errorf("alerts must contain at most %d entries", MaxAlertBatch)
		}
		var alert AlertManagerAlert
		if err := decoder.Decode(&alert); err != nil {
			return fmt.Errorf("decode alert: %w", err)
		}
		alerts = append(alerts, alert)
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("close alerts array: %w", err)
	}
	*a = alerts
	return nil
}
