package fizeau

import (
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/statusview"
)

func statusError(status, source string, capturedAt time.Time) *StatusError {
	return adaptStatusError(statusview.ErrorForStatus(status, source, capturedAt))
}

func statusErrorDetail(status, detail, source string, capturedAt time.Time) *StatusError {
	return adaptStatusError(statusview.ErrorForStatusDetail(status, detail, source, capturedAt))
}

func statusErrorType(status string) string {
	return statusview.ErrorType(status)
}

func quotaStatus(fresh bool, windows []harnesses.QuotaWindow) string {
	return statusview.QuotaStatus(fresh, windows)
}

func accountStatusFromInfo(info *harnesses.AccountInfo, source string, capturedAt time.Time, fresh bool) *AccountStatus {
	return adaptAccountStatus(statusview.AccountFromInfo(info, source, capturedAt, fresh))
}

func providerAuthStatus(entry ServiceProviderEntry, status string, capturedAt time.Time) AccountStatus {
	return adaptAccountStatusValue(statusview.ProviderAuthStatus(statusViewProvider(entry), status, capturedAt))
}

func providerEndpointStatus(entry ServiceProviderEntry, status string, modelCount int, capturedAt time.Time) []EndpointStatus {
	return adaptEndpointStatuses(statusview.ProviderEndpointStatus(statusViewProvider(entry), status, modelCount, capturedAt))
}

func providerQuotaState(entry ServiceProviderEntry, capturedAt time.Time) *QuotaState {
	return adaptQuotaState(statusview.ProviderQuotaState(statusViewProvider(entry), capturedAt))
}

func endpointStatus(status string) string {
	return statusview.EndpointStatusFor(status)
}

func statusViewProvider(entry ServiceProviderEntry) statusview.ServiceProvider {
	endpoints := make([]statusview.ServiceProviderEndpoint, 0, len(entry.Endpoints))
	for _, endpoint := range entry.Endpoints {
		endpoints = append(endpoints, statusview.ServiceProviderEndpoint{
			Name:    endpoint.Name,
			BaseURL: endpoint.BaseURL,
		})
	}
	return statusview.ServiceProvider{
		Type:      normalizeServiceProviderType(entry.Type),
		BaseURL:   entry.BaseURL,
		Endpoints: endpoints,
		APIKey:    entry.APIKey,
	}
}

func adaptStatusError(err *statusview.Error) *StatusError {
	if err == nil {
		return nil
	}
	return &StatusError{
		Type:      err.Type,
		Detail:    err.Detail,
		Source:    err.Source,
		Timestamp: err.Timestamp,
	}
}

func adaptAccountStatus(account *statusview.Account) *AccountStatus {
	if account == nil {
		return nil
	}
	out := adaptAccountStatusValue(*account)
	return &out
}

func adaptAccountStatusValue(account statusview.Account) AccountStatus {
	return AccountStatus{
		Authenticated:   account.Authenticated,
		Unauthenticated: account.Unauthenticated,
		Email:           account.Email,
		PlanType:        account.PlanType,
		OrgName:         account.OrgName,
		Source:          account.Source,
		CapturedAt:      account.CapturedAt,
		Fresh:           account.Fresh,
		Detail:          account.Detail,
	}
}

func adaptEndpointStatuses(endpoints []statusview.Endpoint) []EndpointStatus {
	if len(endpoints) == 0 {
		return nil
	}
	out := make([]EndpointStatus, 0, len(endpoints))
	for _, endpoint := range endpoints {
		out = append(out, EndpointStatus{
			Name:          endpoint.Name,
			BaseURL:       endpoint.BaseURL,
			ProbeURL:      endpoint.ProbeURL,
			Status:        endpoint.Status,
			Source:        endpoint.Source,
			CapturedAt:    endpoint.CapturedAt,
			Fresh:         endpoint.Fresh,
			LastSuccessAt: endpoint.LastSuccessAt,
			ModelCount:    endpoint.ModelCount,
			LastError:     adaptStatusError(endpoint.LastError),
		})
	}
	return out
}

func adaptQuotaState(quota *statusview.Quota) *QuotaState {
	if quota == nil {
		return nil
	}
	return &QuotaState{
		Source:     quota.Source,
		Status:     quota.Status,
		CapturedAt: quota.CapturedAt,
		LastError:  adaptStatusError(quota.LastError),
	}
}
