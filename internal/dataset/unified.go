package dataset

import (
	"context"
	"fmt"
	"strings"

	"github.com/JungHoonGhae/oddsock/internal/apicall"
	"github.com/JungHoonGhae/oddsock/internal/catalog"
	"github.com/JungHoonGhae/oddsock/internal/fetch"
	"github.com/JungHoonGhae/oddsock/internal/portal"
)

// InspectionRequest is the one delivery-neutral request used by CLI and MCP.
// Asset is an exact name returned by a preceding inspection, never a URL.
type DeliverySelection string

const (
	DeliverySelectionAuto DeliverySelection = "auto"
	DeliverySelectionAPI  DeliverySelection = "api"
	DeliverySelectionFile DeliverySelection = "file"
)

type InspectionRequest struct {
	PK       string
	Delivery DeliverySelection // auto (default) | api | file; auto returns every available representation
	Observe  bool
	Asset    string
}

type InspectionResult struct {
	Delivery    string           `json:"delivery"` // primary service type, retained for compatibility
	Deliveries  []string         `json:"deliveries,omitempty"`
	API         *apicall.APISpec `json:"api,omitempty"`
	File        *Contract        `json:"file,omitempty"`
	Observation *Observation     `json:"observation,omitempty"`
}

// UnifiedInspector owns catalogue lookup, delivery dispatch, file asset
// selection and optional observation. User-facing transports only adapt its
// result into CLI JSON or MCP structured content.
type UnifiedInspector struct {
	fetch *fetch.Client
	files *Inspector
	base  string
}

func NewUnifiedInspector(client *fetch.Client, baseURL string) *UnifiedInspector {
	if baseURL == "" {
		baseURL = portal.BaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &UnifiedInspector{fetch: client, files: NewInspector(client, baseURL), base: baseURL}
}

func (i *UnifiedInspector) Inspect(ctx context.Context, request InspectionRequest) (*InspectionResult, error) {
	if i == nil || i.fetch == nil || i.files == nil {
		return nil, fmt.Errorf("dataset inspector가 초기화되지 않았습니다")
	}
	cat, err := catalog.Load()
	if err != nil {
		return nil, err
	}
	entry, ok := cat.Find(request.PK)
	if !ok {
		return nil, fmt.Errorf("현재 카탈로그에 없는 pk입니다 — `oddsock catalog sync` 후 다시 검색하세요")
	}
	wantAPI, wantFile, err := selectedDeliveries(entry, request.Delivery)
	if err != nil {
		return nil, err
	}
	result := &InspectionResult{Delivery: entry.SvcType}
	if wantAPI {
		spec, err := apicall.DescribeCataloguedEntry(ctx, i.fetch, i.base, entry)
		if err != nil {
			return nil, err
		}
		result.API = spec
		result.Deliveries = append(result.Deliveries, "API")
		if !wantFile {
			if spec.APIType != "" {
				result.Delivery = spec.APIType
			} else {
				result.Delivery = "API"
			}
		}
	}
	if !wantFile {
		return result, nil
	}
	contract, err := i.files.Inspect(ctx, Ref{PK: request.PK, Delivery: DeliveryFile})
	if err != nil {
		return nil, err
	}
	result.File = contract
	result.Deliveries = append(result.Deliveries, "FILE")
	if !wantAPI {
		result.Delivery = catalog.SvcFILE
	}
	if !request.Observe {
		return result, nil
	}
	asset, err := selectAsset(contract.Assets, request.Asset)
	if err != nil {
		return nil, err
	}
	result.Observation, err = i.files.Observe(ctx, asset)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func selectedDeliveries(entry catalog.Entry, requested DeliverySelection) (bool, bool, error) {
	hasAPI := entry.SvcType == catalog.SvcREST || entry.SvcType == catalog.SvcLINK || containsFold(entry.DataTypes, "API")
	hasFile := entry.SvcType == catalog.SvcFILE || containsFold(entry.DataTypes, "FILE")
	switch DeliverySelection(strings.ToLower(strings.TrimSpace(string(requested)))) {
	case "", DeliverySelectionAuto, "all":
		if !hasAPI && !hasFile {
			return false, false, fmt.Errorf("pk %s의 제공형을 자동 검사할 수 없습니다 — svcType=%q dataTypes=%v", entry.PK, entry.SvcType, entry.DataTypes)
		}
		return hasAPI, hasFile, nil
	case DeliverySelectionAPI:
		if !hasAPI {
			return false, false, fmt.Errorf("pk %s에는 API 제공형이 없습니다", entry.PK)
		}
		return true, false, nil
	case DeliverySelectionFile:
		if !hasFile {
			return false, false, fmt.Errorf("pk %s에는 FILE 제공형이 없습니다", entry.PK)
		}
		return false, true, nil
	default:
		return false, false, fmt.Errorf("delivery는 auto, api, file 중 하나여야 합니다")
	}
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func selectAsset(assets []Asset, exactName string) (Asset, error) {
	if len(assets) == 0 {
		return Asset{}, fmt.Errorf("관찰할 수 있는 FILE 자산이 없습니다")
	}
	if exactName == "" {
		return assets[0], nil
	}
	for _, asset := range assets {
		if asset.Name == exactName {
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("asset %q을 찾지 못했습니다", exactName)
}
