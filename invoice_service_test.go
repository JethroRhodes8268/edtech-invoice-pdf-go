package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeliverInvoiceAttachesGeneratedPDF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/pdf/generate" {
			_, _ = w.Write([]byte(`{"ok":true,"data":{"pdf":"base64-pdf"}}`))
			return
		}
		if r.URL.Path != "/v1/email/send" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		var request struct {
			Attachments []struct {
				Filename    string `json:"filename"`
				Content     string `json:"content"`
				ContentType string `json:"content_type"`
			} `json:"attachments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode email request: %v", err)
		}
		if len(request.Attachments) != 1 || request.Attachments[0].Content != "base64-pdf" || request.Attachments[0].Filename != "invoice-ed-17.pdf" || request.Attachments[0].ContentType != "application/pdf" {
			t.Errorf("wrong attachments: %+v", request.Attachments)
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":{}}`))
	}))
	defer server.Close()
	client := &InfraiClient{BaseURL: server.URL, Key: "test", HTTP: server.Client()}
	if err := deliverInvoice(client, Order{OrderID: "ed-17", Learner: Learner{Email: "chenhua@changba.com"}}); err != nil {
		t.Fatal(err)
	}
}

func TestInvoiceHTMLBusinessFields(t *testing.T) {
	tests := []struct {
		name  string
		order Order
		want  string
	}{
		{"deadline appears", Order{OrderID: "ed-17", Course: "SQL", DueDate: "2026-10-01", Learner: Learner{Name: "Lin"}}, "2026-10-01"},
		{"amount rendered", Order{OrderID: "ed-18", Course: "Go", AmountCents: 2599, Learner: Learner{Name: "Mo"}}, "$25.99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := invoiceHTML(tt.order); !strings.Contains(got, tt.want) {
				t.Fatalf("invoiceHTML() missing %q: %s", tt.want, got)
			}
		})
	}
}
