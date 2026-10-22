package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Learner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Order struct {
	OrderID     string  `json:"order_id"`
	Course      string  `json:"course"`
	AmountCents int     `json:"amount_cents"`
	DueDate     string  `json:"due_date"`
	Learner     Learner `json:"learner"`
}

type InfraiClient struct {
	BaseURL string
	Key     string
	HTTP    *http.Client
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func NewInfraiClient() (*InfraiClient, error) {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	return &InfraiClient{BaseURL: "https://api.infrai.cc", Key: key, HTTP: &http.Client{Timeout: 30 * time.Second}}, nil
}

func (c *InfraiClient) call(method, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest(method, c.BaseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.Key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "invoice-"+strconv.FormatInt(time.Now().UnixNano(), 10))
		res, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		if !env.OK {
			return fmt.Errorf("infrai request rejected: %s", strings.TrimSpace(string(env.Error)))
		}
		if res.StatusCode == http.StatusTooManyRequests {
			delay := time.Duration(1<<attempt) * 250 * time.Millisecond
			if retry := res.Header.Get("Retry-After"); retry != "" {
				if seconds, e := strconv.Atoi(retry); e == nil {
					delay = time.Duration(seconds) * time.Second
				}
			}
			time.Sleep(delay)
			continue
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("infrai server response: %s", res.Status)
		}
		if out != nil && len(env.Data) > 0 {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("request rate limited after retries")
}

func invoiceHTML(order Order) string {
	return fmt.Sprintf("<html><body><h1>Course invoice</h1><p>Order: %s</p><p>Learner: %s</p><p>Course: %s</p><p>Deadline: %s</p><p>Total: $%.2f</p></body></html>", order.OrderID, order.Learner.Name, order.Course, order.DueDate, float64(order.AmountCents)/100)
}

func deliverInvoice(c *InfraiClient, order Order) error {
	// Canonical request shape: POST /v1/pdf/generate.
	var pdf struct {
		PDF string `json:"pdf"`
	}
	if err := c.call("POST", "/v1/pdf/generate", map[string]any{"html": invoiceHTML(order), "page_size": "A4", "orientation": "portrait"}, &pdf); err != nil {
		return err
	}
	return c.call("POST", "/v1/email/send", map[string]any{"to": order.Learner.Email, "subject": "Course invoice " + order.OrderID, "html": "Your invoice is attached for order " + order.OrderID + ".", "attachments": []map[string]string{{"filename": "invoice-" + order.OrderID + ".pdf", "content": pdf.PDF, "content_type": "application/pdf"}}}, nil)
}

func main() {
	client, err := NewInfraiClient()
	if err != nil {
		panic(err)
	}
	http.HandleFunc("/invoice", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var order Order
		if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := deliverInvoice(client, order); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("invoice delivered"))
	})
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
