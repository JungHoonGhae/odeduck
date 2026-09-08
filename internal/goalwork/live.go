package goalwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/portal"
	"github.com/JungHoonGhae/odeduck/internal/providerauth"
)

type Caller interface {
	Call(context.Context, apicall.DatasetCallRequest) (*apicall.CallResult, error)
}

// LiveDependencies is shared by CLI and MCP. It retains the same catalog
// snapshot for this adapter lifetime and delegates all authenticated access to
// DatasetCaller. It neither applies for access nor accepts endpoint overrides.
func LiveDependencies(client *fetch.Client, base string, caller Caller, searcher catalog.Searcher, policy Policy) Dependencies {
	inspector := dataset.NewUnifiedInspector(client, base)
	files := dataset.NewInspector(client, base)
	var once sync.Once
	var cat *catalog.Catalog
	var loadErr error
	inspect := func(ctx context.Context, pk string, history bool, version string) (Inspection, error) {
		res, err := inspector.Inspect(ctx, dataset.InspectionRequest{PK: pk, FileHistory: history, FileVersion: version})
		if err != nil {
			return Inspection{}, err
		}
		return projectInspection(pk, res), nil
	}
	return Dependencies{
		Search: func(ctx context.Context, query string) (catalog.Result, error) {
			once.Do(func() { cat, loadErr = catalog.Load() })
			if loadErr != nil {
				return catalog.Result{}, loadErr
			}
			return searcher.Search(ctx, cat, catalog.QueryPlan{Intent: query, Limit: maxSearchHits, IncludePreviews: true}, catalog.SearchOptions{Semantic: true, RequireSemantic: policy.RequireSemantic})
		},
		Inspect: func(ctx context.Context, pk string) (Inspection, error) {
			return inspect(ctx, pk, false, "")
		},
		InspectFileVersion: func(ctx context.Context, pk, version string) (Inspection, error) {
			return inspect(ctx, pk, true, version)
		},
		Layout: func(ctx context.Context, r LayoutRequest, i Inspection) (dataset.FileLayout, error) {
			res, ok := i.handle.(*dataset.InspectionResult)
			if !ok || i.PK != r.PK || res.File == nil {
				return dataset.FileLayout{}, fmt.Errorf("missing trusted FILE inspection handle")
			}
			if !selectedFileVersion(r.FileVersion, res.File) {
				return dataset.FileLayout{}, fmt.Errorf("layout fileVersion differs from its inspected contract")
			}
			for _, asset := range res.File.Assets {
				if asset.Name == r.Asset {
					return files.LayoutFile(ctx, asset, r.Sheet)
				}
			}
			return dataset.FileLayout{}, fmt.Errorf("layout asset not in inspected contract")
		},
		ScanCSV: func(ctx context.Context, s SampleRequest, i Inspection, visit func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) {
			if err := validateSampleSelection(s); err != nil {
				return dataset.CSVScanReport{}, "", err
			}
			res, ok := i.handle.(*dataset.InspectionResult)
			if !ok || i.PK != s.PK || res.File == nil || s.Delivery != "file" || !s.ScanCSV {
				return dataset.CSVScanReport{}, "", fmt.Errorf("missing trusted direct CSV scan contract")
			}
			if !selectedFileVersion(s.FileVersion, res.File) {
				return dataset.CSVScanReport{}, "", fmt.Errorf("scan fileVersion differs from its inspected contract")
			}
			for _, asset := range res.File.Assets {
				if asset.Name == s.Asset {
					report, err := files.ScanCSV(ctx, asset, dataset.CSVSelection{Equals: s.Where, In: s.WhereIn}, visit)
					return report, digest(res.File), classifyLiveAcquisitionError(err, nil)
				}
			}
			return dataset.CSVScanReport{}, "", fmt.Errorf("scan asset not in inspected contract")
		},
		Sample: func(ctx context.Context, s SampleRequest, i Inspection) (Acquired, error) {
			if s.Nearest != nil || s.Reduce != nil {
				return Acquired{}, fmt.Errorf("local reduction requires the shared engine's retained observations")
			}
			if err := validateSampleSelection(s); err != nil {
				return Acquired{}, err
			}
			res, ok := i.handle.(*dataset.InspectionResult)
			if !ok || i.PK != s.PK {
				return Acquired{}, fmt.Errorf("missing trusted inspection handle")
			}
			for k := range s.Params {
				if sensitiveParameter(k) {
					return Acquired{}, fmt.Errorf("credential parameters are forbidden; the existing caller supplies scoped keys")
				}
			}
			if s.Delivery == "standard" {
				if len(s.Params) != 0 || s.Operation != "" || s.Asset != "" || s.RowPath != "" {
					return Acquired{}, fmt.Errorf("standard sampling accepts only inspected PK; custom selectors, operation, asset and rowPath are forbidden")
				}
				sample, err := inspector.SampleStandard(ctx, res.Standard, 1000)
				if err != nil {
					return Acquired{}, classifyLiveAcquisitionError(err, nil)
				}
				return Acquired{Rows: sample.Rows, Delivery: "STD", ContentSHA256: sample.SHA256, ContractSHA256: digest(res.Standard), Warnings: append(append([]string(nil), res.Standard.Warnings...), "STD first page only (at most 1000 rows); not representative or population-verified. Original JSON types and record dates are preserved.")}, nil
			}
			if s.Delivery == "document" {
				sample, err := files.SampleDocument(ctx, res.File, *s.Document)
				if err != nil {
					return Acquired{}, classifyLiveAcquisitionError(err, nil)
				}
				return Acquired{Rows: sample.Rows, Delivery: "DOCUMENT", ContentSHA256: sample.SHA256, ContractSHA256: digest(sample.Document.Reference), Document: sample.Document}, nil
			}
			if s.Delivery == "file" {
				if res.File == nil {
					return Acquired{}, fmt.Errorf("no inspected FILE contract")
				}
				if !selectedFileVersion(s.FileVersion, res.File) {
					return Acquired{}, fmt.Errorf("sample fileVersion must match its selected inspected FILE contract")
				}
				if s.Asset == "" {
					return Acquired{}, fmt.Errorf("select an exact inspected asset name")
				}
				for _, asset := range res.File.Assets {
					if asset.Name != s.Asset {
						continue
					}
					var sample dataset.TableSample
					var err error
					if s.XLSX != nil {
						sample, err = files.SampleXLSX(ctx, asset, *s.XLSX)
					} else if s.ScanCSV {
						sample, err = files.SampleCSVScanned(ctx, asset, 1000, dataset.CSVSelection{Equals: s.Where, In: s.WhereIn})
					} else if s.Member != "" {
						sample, err = files.SampleZIPCSV(ctx, asset, s.Member, 1000, s.Where)
					} else {
						sample, err = files.SampleCSVSelected(ctx, asset, 1000, s.Where)
					}
					if err != nil {
						return Acquired{}, classifyLiveAcquisitionError(err, nil)
					}
					out := Acquired{Rows: sample.Rows, Delivery: "FILE", ContentSHA256: sample.SHA256, ContractSHA256: digest(res.File), Warnings: []string{"Direct CSV first-row header; string values are not implicitly converted to numeric identifiers or measures."}}
					if s.XLSX != nil {
						out.Table, out.Warnings = sample.Table, sample.Warnings
					}
					out.Archive, out.CSV = sample.Archive, sample.CSV
					if s.Member != "" || s.ScanCSV {
						out.Warnings = sample.Warnings
					}
					if sample.Prefix {
						out.Warnings = append(out.Warnings, "First 1000 matching rows only (all rows when selectors are absent); ordered prefix is not a representative sample.")
					}
					out.Selection = sample.Selection
					if sample.Selection != nil {
						out.Warnings = append(out.Warnings, "Exact string selection applied before the row limit. Scan coverage is only this downloaded asset, not dataset/population completeness or verified geographic scope.")
					}
					return out, nil
				}
				return Acquired{}, fmt.Errorf("asset not in inspected contract")
			}
			if s.Delivery != "api" || res.API == nil || caller == nil {
				return Acquired{}, fmt.Errorf("no inspected API caller")
			}
			result, err := caller.Call(ctx, apicall.DatasetCallRequest{PK: s.PK, Operation: s.Operation, Params: s.Params})
			if err != nil {
				return Acquired{}, classifyLiveAcquisitionError(err, result)
			}
			if result == nil || result.Status < 200 || result.Status >= 300 {
				return Acquired{}, classifyLiveAcquisitionError(fmt.Errorf("API sample requires a successful response"), result)
			}
			b, err := json.Marshal(result.Body)
			if err != nil || len(b) > 2<<20 {
				return Acquired{}, fmt.Errorf("API response exceeds 2 MiB sampling limit; narrow provider request")
			}
			rows, err := ExtractRows(result.Body, s.RowPath, 1000)
			if err != nil {
				return Acquired{}, err
			}
			return Acquired{Rows: rows, Delivery: result.Delivery, Operation: result.Operation, ContentSHA256: digest(result.Body), ContractSHA256: digest(res.API), Warnings: []string{"Only the requested API response is observed; pagination and population coverage are not inferred. API content hash covers decoded canonical JSON, not transport bytes."}}, nil
		},
	}
}

func projectInspection(pk string, res *dataset.InspectionResult) Inspection {
	out := Inspection{PK: pk, Deliveries: res.Deliveries, Declarations: sourceDeclarations(res), handle: res}
	appendOperation := func(name, title string, params []apicall.Param) {
		if name == "" {
			return
		}
		op := Operation{Name: name, Title: bounded(title, 400)}
		for _, p := range params {
			if sensitiveParameter(p.Name) {
				continue
			}
			op.Parameters = append(op.Parameters, Parameter{Name: p.Name, Required: p.Required, Description: bounded(p.Desc, 400), Example: bounded(p.Sample, 200)})
		}
		out.Operations = append(out.Operations, op)
	}
	if res.API != nil {
		for _, op := range res.API.Operations {
			appendOperation(apicall.OperationName(op), op.Name, op.Params)
		}
		if res.API.Handoff != nil && res.API.Handoff.Contract != nil {
			for _, op := range res.API.Handoff.Contract.Operations {
				appendOperation(op.Name, op.Description, op.Params)
			}
		}
		out.Warnings = append(out.Warnings, res.API.Warnings...)
	}
	if res.File != nil {
		out.Documents = append([]dataset.DocumentReference(nil), res.File.Documents...)
		out.FileVersions, out.FileHistoryCount, out.FileHistoryTruncated, out.SelectedFileVersion = res.File.FileVersions, res.File.FileHistoryCount, res.File.FileHistoryTruncated, res.File.SelectedFileVersion
		for _, a := range res.File.Assets {
			out.Assets = append(out.Assets, a.Name)
		}
		out.Warnings = append(out.Warnings, res.File.Warnings...)
	}
	if res.Standard != nil {
		for _, column := range res.Standard.Columns {
			out.DeclaredColumns = append(out.DeclaredColumns, DeclaredColumn{Field: column.Code, Description: column.Name})
		}
		out.Warnings = append(out.Warnings, res.Standard.Warnings...)
	}
	return out
}

// Classify typed transport/credential evidence, never provider body strings.
// HTTP throttling/server errors stay unclassified: CallResult currently lacks
// Retry-After, so this seam cannot safely offer immediate retries for those.
func classifyLiveAcquisitionError(err error, result *apicall.CallResult) error {
	kind := FailureUnknown
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		kind = FailureCancelled
	case errors.Is(err, portal.ErrNotLoggedIn), errors.Is(err, providerauth.ErrNotConfigured), errors.Is(err, apicall.ErrKeyRejected), errors.Is(err, apicall.ErrPropagating):
		kind = FailureAccessRequired
	case result != nil && result.Status == 401:
		kind = FailureAccessRequired
	default:
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			kind = FailureTransient
		}
	}
	if kind == FailureUnknown {
		return err
	}
	return &AcquisitionError{Kind: kind, Cause: err}
}

func validateSampleSelection(s SampleRequest) error {
	if s.Document != nil || s.Delivery == "document" {
		if s.Document == nil || s.Delivery != "document" || s.FileVersion != "" || s.Reduce != nil || s.Nearest != nil || s.ScanCSV || s.LayoutID != "" || s.Asset != "" || s.Member != "" || s.XLSX != nil || len(s.Where)+len(s.WhereIn)+len(s.Params) != 0 || s.Operation != "" || s.RowPath != "" {
			return fmt.Errorf("document sampling accepts only pk, delivery:document and an inspected document selection")
		}
		return dataset.ValidateDocumentSelection(*s.Document)
	}
	if len(s.FileVersion) > 100 || (s.FileVersion != "" && (s.Delivery != "file" || s.Reduce != nil)) {
		return fmt.Errorf("fileVersion is a bounded inspected FILE acquisition ID; local reductions use their retained source revision")
	}
	if s.Reduce != nil {
		if s.Nearest != nil || s.ScanCSV || s.LayoutID != "" || s.Asset != "" || s.Member != "" || s.XLSX != nil || len(s.Where)+len(s.WhereIn) != 0 || len(s.Params) != 0 || s.Operation != "" || s.RowPath != "" {
			return fmt.Errorf("reduce accepts only pk, original delivery and the retained source reduction; no new acquisition selectors")
		}
		return nil
	}
	if s.Nearest != nil {
		if !s.ScanCSV || s.LayoutID != "" {
			return fmt.Errorf("nearest requires scanCsv:true and its own candidate observation revision pin")
		}
		if err := validateNearestSelection(*s.Nearest); err != nil {
			return err
		}
	}
	if s.ScanCSV && (s.Delivery != "file" || s.Member != "" || s.XLSX != nil) {
		return fmt.Errorf("scanCsv requires a direct FILE CSV without ZIP member or XLSX selection")
	}
	if len(s.WhereIn) != 0 && !s.ScanCSV {
		return fmt.Errorf("whereIn requires a complete direct CSV scan with scanCsv:true; value sets are not silently applied to bounded/ZIP/XLSX/API/STD readers")
	}
	if s.Member != "" {
		if s.Delivery != "file" || s.XLSX != nil {
			return fmt.Errorf("ZIP member selection requires FILE without an XLSX selector")
		}
		if err := dataset.ValidateZIPCSVMember(s.Member); err != nil {
			return err
		}
	}
	if s.XLSX != nil {
		if s.Delivery != "file" || len(s.Where) > 0 {
			return fmt.Errorf("XLSX rectangle selection requires FILE without CSV where predicates")
		}
		if err := dataset.ValidateXLSXSelection(*s.XLSX); err != nil {
			return err
		}
	}
	if len(s.Where) > 0 && s.Delivery != "file" {
		return fmt.Errorf("where selection is supported only for FILE CSV (direct or ZIP member); API/STD filters must not be silently ignored")
	}
	if s.Delivery == "file" && (len(s.Params) > 0 || s.Operation != "" || s.RowPath != "") {
		return fmt.Errorf("FILE sampling uses exact asset, optional ZIP CSV member/where or XLSX rectangle, not API params/operation/rowPath")
	}
	for field := range s.Where {
		if sensitiveParameter(field) {
			return fmt.Errorf("credential selection fields are forbidden")
		}
	}
	for field := range s.WhereIn {
		if sensitiveParameter(field) {
			return fmt.Errorf("credential selection fields are forbidden")
		}
	}
	return (dataset.CSVSelection{Equals: s.Where, In: s.WhereIn}).Validate()
}

func selectedFileVersion(version string, c *dataset.Contract) bool {
	if c.SelectedFileVersion == nil {
		return version == ""
	}
	return version == c.SelectedFileVersion.ID
}

func sensitiveParameter(name string) bool {
	name = strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(name))
	switch name {
	case "key", "servicekey", "apikey", "authkey", "accesskey", "certkey", "token", "accesstoken", "refreshtoken", "password", "secret", "clientsecret", "authorization", "cookie":
		return true
	}
	return false
}
