// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ---------------------------------------------------------------------------
// Datasets (workspaces/{ws}/datasets)
// ---------------------------------------------------------------------------

// Dataset is a knowledge base in the workspace.
type Dataset struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	Description            string          `json:"description"`
	IndexingTechnique      string          `json:"indexing_technique"`
	Permission             string          `json:"permission"`
	Provider               string          `json:"provider"`
	EmbeddingModel         string          `json:"embedding_model"`
	EmbeddingModelProvider string          `json:"embedding_model_provider"`
	RetrievalModel         json.RawMessage `json:"retrieval_model"`
	ExternalKnowledgeAPIId string          `json:"external_knowledge_api_id"`
	ExternalKnowledgeId    string          `json:"external_knowledge_id"`
	CreatedAt              int64           `json:"created_at"`
	UpdatedAt              int64           `json:"updated_at"`
}

type datasetListResponse struct {
	HasMore bool      `json:"has_more"`
	Data    []Dataset `json:"data"`
}

// DatasetCreateRequest is the payload for POST /datasets.
type DatasetCreateRequest struct {
	Name                   string           `json:"name"`
	Description            string           `json:"description,omitempty"`
	IndexingTechnique      *string          `json:"indexing_technique,omitempty"`
	Permission             *string          `json:"permission,omitempty"`
	Provider               *string          `json:"provider,omitempty"`
	ExternalKnowledgeAPIId *string          `json:"external_knowledge_api_id,omitempty"`
	ExternalKnowledgeId    *string          `json:"external_knowledge_id,omitempty"`
	RetrievalModel         *json.RawMessage `json:"retrieval_model,omitempty"`
	EmbeddingModel         *string          `json:"embedding_model,omitempty"`
	EmbeddingModelProvider *string          `json:"embedding_model_provider,omitempty"`
	SummaryIndexSetting    *json.RawMessage `json:"summary_index_setting,omitempty"`
}

// DatasetUpdateRequest is the payload for PATCH /datasets/{id}.
type DatasetUpdateRequest struct {
	Name                   *string             `json:"name,omitempty"`
	Description            *string             `json:"description,omitempty"`
	IndexingTechnique      *string             `json:"indexing_technique,omitempty"`
	Permission             *string             `json:"permission,omitempty"`
	Provider               *string             `json:"provider,omitempty"`
	EmbeddingModel         *string             `json:"embedding_model,omitempty"`
	EmbeddingModelProvider *string             `json:"embedding_model_provider,omitempty"`
	RetrievalModel         *json.RawMessage    `json:"retrieval_model,omitempty"`
	PartialMemberList      []map[string]string `json:"partial_member_list,omitempty"`
	ExternalRetrievalModel *json.RawMessage    `json:"external_retrieval_model,omitempty"`
	ExternalKnowledgeId    *string             `json:"external_knowledge_id,omitempty"`
	ExternalKnowledgeAPIId *string             `json:"external_knowledge_api_id,omitempty"`
}

// CreateDataset creates a knowledge base (201 with the dataset payload).
func (c *DifyClient) CreateDataset(ctx context.Context, req DatasetCreateRequest) (*Dataset, error) {
	var result Dataset
	if err := c.doJSON(ctx, http.MethodPost, c.workspaceURL("/datasets"), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDataset reads one knowledge base by id.
func (c *DifyClient) GetDataset(ctx context.Context, datasetID string) (*Dataset, error) {
	var result Dataset
	if err := c.doJSON(ctx, http.MethodGet, c.workspaceURL("/datasets/"+url.PathEscape(datasetID)), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateDataset replaces a knowledge base by id.
func (c *DifyClient) UpdateDataset(ctx context.Context, datasetID string, req DatasetUpdateRequest) (*Dataset, error) {
	var result Dataset
	if err := c.doJSON(ctx, http.MethodPatch, c.workspaceURL("/datasets/"+url.PathEscape(datasetID)), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteDataset deletes a knowledge base by id.
func (c *DifyClient) DeleteDataset(ctx context.Context, datasetID string) error {
	return c.doJSON(ctx, http.MethodDelete, c.workspaceURL("/datasets/"+url.PathEscape(datasetID)), nil, nil)
}

// ListDatasetIDs returns every knowledge base id in the workspace, following
// pagination. An optional name filter can be applied server-side via keyword.
func (c *DifyClient) ListDatasetIDs(ctx context.Context) ([]string, error) {
	var ids []string
	for page := 1; ; page++ {
		values := url.Values{}
		values.Set("page", strconv.Itoa(page))
		values.Set("limit", "100")

		urlStr := c.workspaceURL("/datasets") + "?" + values.Encode()
		var result datasetListResponse
		if err := c.doJSON(ctx, http.MethodGet, urlStr, nil, &result); err != nil {
			return nil, err
		}

		for _, d := range result.Data {
			ids = append(ids, d.ID)
		}
		if !result.HasMore {
			return ids, nil
		}
	}
}

// ---------------------------------------------------------------------------
// Documents (workspaces/{ws}/datasets/{dataset_id}/documents)
// ---------------------------------------------------------------------------

// Document is one document in a knowledge base.
type Document struct {
	ID                   string          `json:"id"`
	DatasetID            string          `json:"dataset_id"`
	Name                 string          `json:"name"`
	DataSourceType       string          `json:"data_source_type"`
	DataSourceInfo       json.RawMessage `json:"data_source_info"`
	DatasetProcessRuleID string          `json:"dataset_process_rule_id"`
	DatasetProcessRule   json.RawMessage `json:"dataset_process_rule"`
	DocumentProcessRule  json.RawMessage `json:"document_process_rule"`
	CreatedFrom          string          `json:"created_from"`
	CreatedBy            string          `json:"created_by"`
	CreatedAt            int64           `json:"created_at"`
	Tokens               int             `json:"tokens"`
	IndexingStatus       string          `json:"indexing_status"`
	CompletedAt          *int64          `json:"completed_at"`
	UpdatedAt            int64           `json:"updated_at"`
	IndexingLatency      float64         `json:"indexing_latency"`
	Error                string          `json:"error"`
	Enabled              bool            `json:"enabled"`
	DisabledAt           *int64          `json:"disabled_at"`
	DisabledBy           string          `json:"disabled_by"`
	Archived             bool            `json:"archived"`
	DocType              string          `json:"doc_type"`
	DocMetadata          json.RawMessage `json:"doc_metadata"`
	SegmentCount         int             `json:"segment_count"`
	AverageSegmentLength interface{}     `json:"average_segment_length"`
	HitCount             int             `json:"hit_count"`
	DisplayStatus        string          `json:"display_status"`
	DocForm              string          `json:"doc_form"`
	DocLanguage          string          `json:"doc_language"`
	SummaryIndexStatus   string          `json:"summary_index_status"`
	NeedSummary          bool            `json:"need_summary"`
}

type documentListResponse struct {
	HasMore bool       `json:"has_more"`
	Data    []Document `json:"data"`
}

// DocumentCreateRequest is the payload for POST /datasets/{dataset_id}/document/create-by-text.
type DocumentCreateRequest struct {
	Name                   string           `json:"name"`
	Text                   string           `json:"text"`
	ProcessRule            *json.RawMessage `json:"process_rule,omitempty"`
	DocForm                *string          `json:"doc_form,omitempty"`
	DocLanguage            *string          `json:"doc_language,omitempty"`
	IndexingTechnique      *string          `json:"indexing_technique,omitempty"`
	RetrievalModel         *json.RawMessage `json:"retrieval_model,omitempty"`
	EmbeddingModel         *string          `json:"embedding_model,omitempty"`
	EmbeddingModelProvider *string          `json:"embedding_model_provider,omitempty"`
}

// DocumentUpdateRequest is the payload for POST /datasets/{dataset_id}/documents/{document_id}/update-by-text.
type DocumentUpdateRequest struct {
	Name           *string          `json:"name,omitempty"`
	Text           *string          `json:"text,omitempty"`
	ProcessRule    *json.RawMessage `json:"process_rule,omitempty"`
	DocForm        *string          `json:"doc_form,omitempty"`
	DocLanguage    *string          `json:"doc_language,omitempty"`
	RetrievalModel *json.RawMessage `json:"retrieval_model,omitempty"`
}

// DocumentAndBatch is the response of create-by-text / update-by-text.
type DocumentAndBatch struct {
	Document Document `json:"document"`
	Batch    string   `json:"batch"`
}

// CreateDocument creates a document from text (200 with document + batch).
func (c *DifyClient) CreateDocument(ctx context.Context, datasetID string, req DocumentCreateRequest) (*DocumentAndBatch, error) {
	var result DocumentAndBatch
	if err := c.doJSON(ctx, http.MethodPost, c.workspaceURL("/datasets/"+url.PathEscape(datasetID)+"/document/create-by-text"), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDocument reads one document by id within a dataset.
func (c *DifyClient) GetDocument(ctx context.Context, datasetID, documentID string) (*Document, error) {
	var result Document
	if err := c.doJSON(ctx, http.MethodGet, c.workspaceURL("/datasets/"+url.PathEscape(datasetID)+"/documents/"+url.PathEscape(documentID)), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateDocument updates a document's text/config by id within a dataset.
func (c *DifyClient) UpdateDocument(ctx context.Context, datasetID, documentID string, req DocumentUpdateRequest) (*DocumentAndBatch, error) {
	var result DocumentAndBatch
	if err := c.doJSON(ctx, http.MethodPost, c.workspaceURL("/datasets/"+url.PathEscape(datasetID)+"/documents/"+url.PathEscape(documentID)+"/update-by-text"), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteDocument deletes a document by id within a dataset.
func (c *DifyClient) DeleteDocument(ctx context.Context, datasetID, documentID string) error {
	return c.doJSON(ctx, http.MethodDelete, c.workspaceURL("/datasets/"+url.PathEscape(datasetID)+"/documents/"+url.PathEscape(documentID)), nil, nil)
}

// ListDocuments returns every document in a dataset, following pagination.
func (c *DifyClient) ListDocuments(ctx context.Context, datasetID string) ([]Document, error) {
	var all []Document
	for page := 1; ; page++ {
		values := url.Values{}
		values.Set("page", strconv.Itoa(page))
		values.Set("limit", "100")

		urlStr := c.workspaceURL("/datasets/"+url.PathEscape(datasetID)+"/documents") + "?" + values.Encode()
		var result documentListResponse
		if err := c.doJSON(ctx, http.MethodGet, urlStr, nil, &result); err != nil {
			return nil, err
		}

		all = append(all, result.Data...)
		if !result.HasMore {
			return all, nil
		}
	}
}

// DocumentIndexingStatus is the response of GET /documents/<batch>/indexing-status.
type DocumentIndexingStatus struct {
	Batch          string `json:"batch"`
	IndexingStatus string `json:"indexing_status"`
	Error          string `json:"error"`
}

// GetDocumentIndexingStatus returns the indexing status of a document batch.
func (c *DifyClient) GetDocumentIndexingStatus(ctx context.Context, datasetID, batch string) (*DocumentIndexingStatus, error) {
	var result DocumentIndexingStatus
	if err := c.doJSON(ctx, http.MethodGet, c.workspaceURL("/datasets/"+url.PathEscape(datasetID)+"/documents/"+url.PathEscape(batch)+"/indexing-status"), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WaitForDocumentIndexing blocks until the document's indexing reaches
// "available" or "failed", honouring ctx cancellation and the given timeout.
func (c *DifyClient) WaitForDocumentIndexing(ctx context.Context, datasetID, documentID string, timeout, interval time.Duration) (*Document, error) {
	deadline := time.Now().Add(timeout)
	for {
		doc, err := c.GetDocument(ctx, datasetID, documentID)
		if err != nil {
			return nil, err
		}

		switch doc.IndexingStatus {
		case "available":
			return doc, nil
		case "failed":
			return nil, fmt.Errorf("document indexing failed: %s", doc.Error)
		}

		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("document indexing did not finish within %s", timeout)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}
