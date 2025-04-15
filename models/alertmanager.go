package models

type AlertManagerAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt"`
}

type AlertManagerRequest struct {
	Receiver string              `json:"receiver"`
	Status   string              `json:"status"`
	Alerts   []AlertManagerAlert `json:"alerts"`
}
