package stripe

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	stripego "github.com/stripe/stripe-go/v85"
)

// InvoiceListItem is the JSON shape returned by GET /api/stripe/invoices.
type InvoiceListItem struct {
	ID               string `json:"id"`
	StripeInvoiceID  string `json:"stripe_invoice_id"`
	InvoiceNumber    string `json:"invoice_number"`
	Status           string `json:"status"`
	Currency         string `json:"currency"`
	AmountDue        int64  `json:"amount_due"`
	AmountPaid       int64  `json:"amount_paid"`
	PeriodStart      string `json:"period_start,omitempty"`
	PeriodEnd        string `json:"period_end,omitempty"`
	InvoiceCreated   string `json:"invoice_created,omitempty"`
	FinalizedAt      string `json:"finalized_at,omitempty"`
	PaidAt           string `json:"paid_at,omitempty"`
	Description      string `json:"description,omitempty"`
	HostedInvoiceURL string `json:"hosted_invoice_url,omitempty"`
	PDFDownloadURL   string `json:"pdf_download_url"`
	Created          string `json:"created"`
}

// InvoiceListResponse wraps paginated invoice results.
type InvoiceListResponse struct {
	Items      []InvoiceListItem `json:"items"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
	TotalItems int               `json:"total_items"`
	TotalPages int               `json:"total_pages"`
}

// SyncInvoices fetches all invoices for the user's Stripe customer and
// upserts them into the local stripe_invoices collection.  This is called
// lazily when the user requests their invoice list.
func (m *Manager) SyncInvoices(userID string) error {
	if !m.IsConfigured() {
		return ErrNotConfigured
	}

	customerID, err := m.getStripeCustomerID(userID)
	if err != nil || customerID == "" {
		return nil // no Stripe customer → nothing to sync
	}

	ctx := context.Background()
	params := &stripego.InvoiceListParams{
		Customer: stripego.String(customerID),
	}
	params.Limit = stripego.Int64(100) // fetch up to 100 most recent

	iter := m.sc.V1Invoices.List(ctx, params)

	col, err := m.app.FindCollectionByNameOrId("stripe_invoices")
	if err != nil {
		return fmt.Errorf("stripe: stripe_invoices collection not found: %w", err)
	}

	for invoice, err := range iter.All(ctx) {
		if err != nil {
			return fmt.Errorf("stripe: list invoices: %w", err)
		}
		if err := m.upsertInvoice(col, userID, invoice); err != nil {
			log.Printf("[stripe] WARNING: failed to upsert invoice %s: %v", invoice.ID, err)
			continue
		}
	}

	return nil
}

// GetInvoices returns a paginated list of invoices from the local DB.
func (m *Manager) GetInvoices(userID string, page, perPage int) (*InvoiceListResponse, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 10
	}

	// Count total
	allRecords, err := m.app.FindAllRecords("stripe_invoices",
		dbx.NewExp("user_id = {:uid}", dbx.Params{"uid": userID}),
	)
	if err != nil {
		return &InvoiceListResponse{Items: []InvoiceListItem{}, Page: page, PerPage: perPage}, nil
	}

	totalItems := len(allRecords)
	totalPages := (totalItems + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}

	// Manual pagination with sorting (newest first)
	// PocketBase FindAllRecords doesn't support LIMIT/OFFSET directly with
	// filter expressions, so we sort and slice manually.
	// Sort by invoice_created descending
	sortInvoiceRecords(allRecords)

	start := (page - 1) * perPage
	if start >= totalItems {
		return &InvoiceListResponse{
			Items:      []InvoiceListItem{},
			Page:       page,
			PerPage:    perPage,
			TotalItems: totalItems,
			TotalPages: totalPages,
		}, nil
	}
	end := start + perPage
	if end > totalItems {
		end = totalItems
	}
	pageRecords := allRecords[start:end]

	items := make([]InvoiceListItem, 0, len(pageRecords))
	for _, rec := range pageRecords {
		items = append(items, invoiceRecordToListItem(rec))
	}

	return &InvoiceListResponse{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}, nil
}

// GetInvoicePDFURL retrieves the PDF download URL for a specific invoice
// from Stripe.  PDF URLs are short-lived so we fetch them on demand.
func (m *Manager) GetInvoicePDFURL(userID, invoiceRecordID string) (string, error) {
	if !m.IsConfigured() {
		return "", ErrNotConfigured
	}

	// Find the local record
	records, err := m.app.FindAllRecords("stripe_invoices",
		dbx.NewExp("id = {:id} AND user_id = {:uid}", dbx.Params{
			"id":  invoiceRecordID,
			"uid": userID,
		}),
	)
	if err != nil || len(records) == 0 {
		return "", ErrInvoiceNotFound
	}

	stripeInvoiceID := records[0].GetString("stripe_invoice_id")

	// Fetch fresh from Stripe to get current PDF URL
	ctx := context.Background()
	var invoice *stripego.Invoice
	if retryErr := retryDo(defaultRetry, func() error {
		var getErr error
		invoice, getErr = m.sc.V1Invoices.Retrieve(ctx, stripeInvoiceID, &stripego.InvoiceRetrieveParams{})
		return getErr
	}); retryErr != nil {
		return "", fmt.Errorf("stripe: retrieve invoice: %w", retryErr)
	}

	if invoice.InvoicePDF == "" {
		return "", fmt.Errorf("stripe: invoice %s has no PDF available", stripeInvoiceID)
	}

	return invoice.InvoicePDF, nil
}

// upsertInvoice creates or updates a local invoice record.
func (m *Manager) upsertInvoice(col *core.Collection, userID string, invoice *stripego.Invoice) error {
	// Check if already exists
	records, _ := m.app.FindAllRecords("stripe_invoices",
		dbx.NewExp("stripe_invoice_id = {:sid}", dbx.Params{"sid": invoice.ID}),
	)

	var rec *core.Record
	if len(records) > 0 {
		rec = records[0]
	} else {
		rec = core.NewRecord(col)
		rec.Set("user_id", userID)
		rec.Set("stripe_invoice_id", invoice.ID)
	}

	customerID := ""
	if invoice.Customer != nil {
		customerID = invoice.Customer.ID
	}
	subID := extractInvoiceSubscriptionID(invoice)

	rec.Set("stripe_customer_id", customerID)
	rec.Set("stripe_subscription_id", subID)
	rec.Set("invoice_number", invoice.Number)
	rec.Set("status", string(invoice.Status))
	rec.Set("currency", string(invoice.Currency))
	rec.Set("amount_due", invoice.AmountDue)
	rec.Set("amount_paid", invoice.AmountPaid)

	if invoice.PeriodStart != 0 {
		rec.Set("period_start", time.Unix(invoice.PeriodStart, 0).UTC().Format(time.RFC3339))
	}
	if invoice.PeriodEnd != 0 {
		rec.Set("period_end", time.Unix(invoice.PeriodEnd, 0).UTC().Format(time.RFC3339))
	}
	if invoice.Created != 0 {
		rec.Set("invoice_created", time.Unix(invoice.Created, 0).UTC().Format(time.RFC3339))
	}
	if invoice.StatusTransitions != nil && invoice.StatusTransitions.FinalizedAt != 0 {
		rec.Set("finalized_at", time.Unix(invoice.StatusTransitions.FinalizedAt, 0).UTC().Format(time.RFC3339))
	}
	if invoice.StatusTransitions != nil && invoice.StatusTransitions.PaidAt != 0 {
		rec.Set("paid_at", time.Unix(invoice.StatusTransitions.PaidAt, 0).UTC().Format(time.RFC3339))
	}

	// Use first line item description as summary
	if len(invoice.Lines.Data) > 0 && invoice.Lines.Data[0].Description != "" {
		rec.Set("description", invoice.Lines.Data[0].Description)
	}

	rec.Set("hosted_invoice_url", invoice.HostedInvoiceURL)

	return m.app.Save(rec)
}

// sortInvoiceRecords sorts records by invoice_created descending (newest first).
func sortInvoiceRecords(records []*core.Record) {
	for i := 1; i < len(records); i++ {
		for j := i; j > 0; j-- {
			a := records[j].GetDateTime("invoice_created")
			b := records[j-1].GetDateTime("invoice_created")
			if a.Time().After(b.Time()) {
				records[j], records[j-1] = records[j-1], records[j]
			} else {
				break
			}
		}
	}
}

// invoiceRecordToListItem maps a PocketBase record to the API response shape.
func invoiceRecordToListItem(rec *core.Record) InvoiceListItem {
	return InvoiceListItem{
		ID:               rec.Id,
		StripeInvoiceID:  rec.GetString("stripe_invoice_id"),
		InvoiceNumber:    rec.GetString("invoice_number"),
		Status:           rec.GetString("status"),
		Currency:         rec.GetString("currency"),
		AmountDue:        int64(rec.GetInt("amount_due")),
		AmountPaid:       int64(rec.GetInt("amount_paid")),
		PeriodStart:      rec.GetString("period_start"),
		PeriodEnd:        rec.GetString("period_end"),
		InvoiceCreated:   rec.GetString("invoice_created"),
		FinalizedAt:      rec.GetString("finalized_at"),
		PaidAt:           rec.GetString("paid_at"),
		Description:      rec.GetString("description"),
		HostedInvoiceURL: rec.GetString("hosted_invoice_url"),
		// PDF URL is built by the route handler (requires the base URL)
		PDFDownloadURL: fmt.Sprintf("/api/stripe/invoices/%s/pdf", rec.Id),
		Created:        rec.GetString("created"),
	}
}
