package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/net/http/httpproxy"
)

type modelV0 struct {
	ID                 types.String `tfsdk:"id"`
	URL                types.String `tfsdk:"url"`
	Method             types.String `tfsdk:"method"`
	RequestHeaders     types.Map    `tfsdk:"request_headers"`
	RequestBody        types.String `tfsdk:"request_body"`
	RequestTimeout     types.Int64  `tfsdk:"request_timeout_ms"`
	Retry              types.Object `tfsdk:"retry"`
	ResponseHeaders    types.Map    `tfsdk:"response_headers"`
	CaCertificate      types.String `tfsdk:"ca_cert_pem"`
	ClientCert         types.String `tfsdk:"client_cert_pem"`
	ClientKey          types.String `tfsdk:"client_key_pem"`
	Insecure           types.Bool   `tfsdk:"insecure"`
	ResponseBody       types.String `tfsdk:"response_body"`
	Body               types.String `tfsdk:"body"`
	ResponseBodyBase64 types.String `tfsdk:"response_body_base64"`
	StatusCode         types.Int64  `tfsdk:"status_code"`
	SuccessStatusCodes types.List   `tfsdk:"success_status_codes"`
}

type retryModel struct {
	Attempts types.Int64 `tfsdk:"attempts"`
	MinDelay types.Int64 `tfsdk:"min_delay_ms"`
	MaxDelay types.Int64 `tfsdk:"max_delay_ms"`
}

var _ retryablehttp.LeveledLogger = levelledLogger{}

// levelledLogger is used to log messages from retryablehttp.Client to tflog.
type levelledLogger struct {
	ctx context.Context
}

func (l levelledLogger) Error(msg string, keysAndValues ...interface{}) {
	tflog.Error(l.ctx, msg, l.additionalFields(keysAndValues))
}

func (l levelledLogger) Info(msg string, keysAndValues ...interface{}) {
	tflog.Info(l.ctx, msg, l.additionalFields(keysAndValues))
}

func (l levelledLogger) Debug(msg string, keysAndValues ...interface{}) {
	tflog.Debug(l.ctx, msg, l.additionalFields(keysAndValues))
}

func (l levelledLogger) Warn(msg string, keysAndValues ...interface{}) {
	tflog.Warn(l.ctx, msg, l.additionalFields(keysAndValues))
}

func (l levelledLogger) additionalFields(keysAndValues []interface{}) map[string]interface{} {
	additionalFields := make(map[string]interface{}, len(keysAndValues))

	for i := 0; i+1 < len(keysAndValues); i += 2 {
		additionalFields[fmt.Sprint(keysAndValues[i])] = keysAndValues[i+1]
	}

	return additionalFields
}

func makeCustomRetryPolicy(successStatusCodes []int) retryablehttp.CheckRetry {
	return func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}

		shouldRetry, err2 := retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		if !shouldRetry && err2 != nil {
			return false, err2
		}

		// The error is likely recoverable so retry.
		if err != nil {
			return true, nil
		}

		if len(successStatusCodes) == 0 {
			return shouldRetry, err2
		}

		for _, code := range successStatusCodes {
			if resp.StatusCode == code {
				return false, nil
			}
		}

		return true, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}
}

type Diags struct {
	Diagnostics diag.Diagnostics
}

func (model *modelV0) read(ctx context.Context, diagnostics *diag.Diagnostics) {
	requestURL := model.URL.ValueString()
	method := model.Method.ValueString()
	requestHeaders := model.RequestHeaders

	if method == "" {
		method = "GET"
	}

	caCertificate := model.CaCertificate

	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		diagnostics.AddError(
			"Error configuring http transport",
			"Error http: Can't configure http transport.",
		)
		return
	}

	// Prevent issues with multiple data source configurations modifying the shared transport.
	clonedTr := tr.Clone()

	// Prevent issues with tests caching the proxy configuration.
	clonedTr.Proxy = func(req *http.Request) (*url.URL, error) {
		return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
	}

	if clonedTr.TLSClientConfig == nil {
		clonedTr.TLSClientConfig = &tls.Config{}
	}

	if !model.Insecure.IsNull() {
		if clonedTr.TLSClientConfig == nil {
			clonedTr.TLSClientConfig = &tls.Config{}
		}
		clonedTr.TLSClientConfig.InsecureSkipVerify = model.Insecure.ValueBool()
	}

	// Use `ca_cert_pem` cert pool
	if !caCertificate.IsNull() {
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM([]byte(caCertificate.ValueString())); !ok {
			diagnostics.AddError(
				"Error configuring TLS client",
				"Error tls: Can't add the CA certificate to certificate pool. Only PEM encoded certificates are supported.",
			)
			return
		}

		if clonedTr.TLSClientConfig == nil {
			clonedTr.TLSClientConfig = &tls.Config{}
		}
		clonedTr.TLSClientConfig.RootCAs = caCertPool
	}

	if !model.ClientCert.IsNull() && !model.ClientKey.IsNull() {
		cert, err := tls.X509KeyPair([]byte(model.ClientCert.ValueString()), []byte(model.ClientKey.ValueString()))
		if err != nil {
			diagnostics.AddError(
				"error creating x509 key pair",
				fmt.Sprintf("error creating x509 key pair from provided pem blocks\n\nError: %s", err),
			)
			return
		}
		clonedTr.TLSClientConfig.Certificates = []tls.Certificate{cert}
	}

	var retry retryModel

	if !model.Retry.IsNull() && !model.Retry.IsUnknown() {
		diags := model.Retry.As(ctx, &retry, basetypes.ObjectAsOptions{})
		diagnostics.Append(diags...)
		if diagnostics.HasError() {
			return
		}
	}

	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient.Transport = clonedTr

	var timeout time.Duration

	if model.RequestTimeout.ValueInt64() > 0 {
		timeout = time.Duration(model.RequestTimeout.ValueInt64()) * time.Millisecond
		retryClient.HTTPClient.Timeout = timeout
	}

	retryClient.Logger = levelledLogger{ctx}
	retryClient.RetryMax = int(retry.Attempts.ValueInt64())

	var successStatusCodes []int
	if !model.SuccessStatusCodes.IsNull() && !model.SuccessStatusCodes.IsUnknown() {
		diags := model.SuccessStatusCodes.ElementsAs(ctx, &successStatusCodes, false)
		diagnostics.Append(diags...)
	}

	if !retry.MinDelay.IsNull() && !retry.MinDelay.IsUnknown() && retry.MinDelay.ValueInt64() >= 0 {
		retryClient.RetryWaitMin = time.Duration(retry.MinDelay.ValueInt64()) * time.Millisecond
	}

	if !retry.MaxDelay.IsNull() && !retry.MaxDelay.IsUnknown() && retry.MaxDelay.ValueInt64() >= 0 {
		retryClient.RetryWaitMax = time.Duration(retry.MaxDelay.ValueInt64()) * time.Millisecond
	}

	retryClient.CheckRetry = makeCustomRetryPolicy(successStatusCodes)
	request, err := retryablehttp.NewRequestWithContext(ctx, method, requestURL, nil)

	if err != nil {
		diagnostics.AddError(
			"Error creating request",
			fmt.Sprintf("Error creating request: %s", err),
		)
		return
	}

	if !model.RequestBody.IsNull() {
		err = request.SetBody(strings.NewReader(model.RequestBody.ValueString()))

		if err != nil {
			diagnostics.AddError(
				"Error Setting Request Body",
				"An unexpected error occurred while setting the request body: "+err.Error(),
			)

			return
		}
	}

	for name, value := range requestHeaders.Elements() {
		var header string
		diags := tfsdk.ValueAs(ctx, value, &header)
		diagnostics.Append(diags...)
		if diagnostics.HasError() {
			return
		}

		request.Header.Set(name, header)
		if strings.ToLower(name) == "host" {
			request.Host = header
		}
	}

	response, err := retryClient.Do(request)
	if err != nil {
		target := &url.Error{}
		if errors.As(err, &target) {
			if target.Timeout() {
				detail := fmt.Sprintf("timeout error: %s", err)

				if timeout > 0 {
					detail = fmt.Sprintf("request exceeded the specified timeout: %s, err: %s", timeout.String(), err)
				}

				diagnostics.AddError(
					"Error making request",
					detail,
				)
				return
			}
		}

		diagnostics.AddError(
			"Error making request",
			fmt.Sprintf("Error making request: %s", err),
		)
		return
	}

	defer response.Body.Close()

	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		diagnostics.AddError(
			"Error reading response body",
			fmt.Sprintf("Error reading response body: %s", err),
		)
		return
	}

	if !utf8.Valid(bytes) {
		diagnostics.AddWarning(
			"Response body is not recognized as UTF-8",
			"Terraform may not properly handle the response_body if the contents are binary.",
		)
	}

	responseBody := string(bytes)
	responseBodyBase64Std := base64.StdEncoding.EncodeToString(bytes)

	responseHeaders := make(map[string]string)
	for k, v := range response.Header {
		// Concatenate according to RFC9110 https://www.rfc-editor.org/rfc/rfc9110.html#section-5.2
		responseHeaders[k] = strings.Join(v, ", ")
	}

	respHeadersState, diags := types.MapValueFrom(ctx, types.StringType, responseHeaders)
	diagnostics.Append(diags...)
	if diagnostics.HasError() {
		return
	}

	model.ID = types.StringValue(requestURL)
	model.ResponseHeaders = respHeadersState
	model.ResponseBody = types.StringValue(responseBody)
	model.Body = types.StringValue(responseBody)
	model.ResponseBodyBase64 = types.StringValue(responseBodyBase64Std)
	model.StatusCode = types.Int64Value(int64(response.StatusCode))
}
