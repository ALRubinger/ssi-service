package router

import (
	"context"
	"fmt"
	"net/http"

	credsdk "github.com/TBD54566975/ssi-sdk/credential"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/tbd54566975/ssi-service/internal/util"
	"github.com/tbd54566975/ssi-service/pkg/server/framework"
	"github.com/tbd54566975/ssi-service/pkg/service/credential"
	svcframework "github.com/tbd54566975/ssi-service/pkg/service/framework"
)

const (
	IssuerParam  string = "issuer"
	SubjectParam string = "subject"
	SchemaParam  string = "schema"
)

type CredentialRouter struct {
	service *credential.Service
}

func NewCredentialRouter(s svcframework.Service) (*CredentialRouter, error) {
	if s == nil {
		return nil, errors.New("service cannot be nil")
	}
	credService, ok := s.(*credential.Service)
	if !ok {
		return nil, fmt.Errorf("could not create credential router with service type: %s", s.Type())
	}
	return &CredentialRouter{
		service: credService,
	}, nil
}

type CreateCredentialRequest struct {
	Issuer  string `json:"issuer" validate:"required"`
	Subject string `json:"subject" validate:"required"`
	// A context is optional. If not present, we'll apply default, required context values.
	Context string `json:"@context"`
	// A schema is optional. If present, we'll attempt to look it up and validate the data against it.
	Schema string                 `json:"schema"`
	Data   map[string]interface{} `json:"data" validate:"required"`
	Expiry string                 `json:"expiry"`
	// TODO(gabe) support more capabilities like signature type, format, status, and more.
}

func (c CreateCredentialRequest) ToServiceRequest() credential.CreateCredentialRequest {
	return credential.CreateCredentialRequest{
		Issuer:     c.Issuer,
		Subject:    c.Subject,
		Context:    c.Context,
		JSONSchema: c.Schema,
		Data:       c.Data,
		Expiry:     c.Expiry,
	}
}

type CreateCredentialResponse struct {
	Credential credsdk.VerifiableCredential `json:"credential"`
}

// CreateCredential godoc
// @Summary      Create Credential
// @Description  Create credential
// @Tags         CredentialAPI
// @Accept       json
// @Produce      json
// @Param        request  body      CreateCredentialRequest  true  "request body"
// @Success      201      {object}  CreateCredentialResponse
// @Failure      400      {string}  string  "Bad request"
// @Failure      500      {string}  string  "Internal server error"
// @Router       /v1/credentials [put]
func (cr CredentialRouter) CreateCredential(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	var request CreateCredentialRequest
	if err := framework.Decode(r, &request); err != nil {
		errMsg := "invalid create credential request"
		logrus.WithError(err).Error(errMsg)
		return framework.NewRequestError(errors.Wrap(err, errMsg), http.StatusBadRequest)
	}

	req := request.ToServiceRequest()
	createCredentialResponse, err := cr.service.CreateCredential(req)
	if err != nil {
		errMsg := "could not create credential"
		logrus.WithError(err).Error(errMsg)
		return framework.NewRequestError(errors.Wrap(err, errMsg), http.StatusInternalServerError)
	}

	resp := CreateCredentialResponse{Credential: createCredentialResponse.Credential}
	return framework.Respond(ctx, w, resp, http.StatusCreated)
}

type GetCredentialResponse struct {
	ID         string                       `json:"id"`
	Credential credsdk.VerifiableCredential `json:"credential"`
}

// GetCredential godoc
// @Summary      Get Credential
// @Description  Get credential by id
// @Tags         CredentialAPI
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "ID"
// @Success      200  {object}  GetCredentialResponse
// @Failure      400  {string}  string  "Bad request"
// @Router       /v1/credentials/{id} [get]
func (cr CredentialRouter) GetCredential(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	id := framework.GetParam(ctx, IDParam)
	if id == nil {
		errMsg := "cannot get credential without ID parameter"
		logrus.Error(errMsg)
		return framework.NewRequestErrorMsg(errMsg, http.StatusBadRequest)
	}

	gotCredential, err := cr.service.GetCredential(credential.GetCredentialRequest{ID: *id})
	if err != nil {
		errMsg := fmt.Sprintf("could not get credential with id: %s", *id)
		logrus.WithError(err).Error(errMsg)
		return framework.NewRequestError(errors.Wrap(err, errMsg), http.StatusBadRequest)
	}

	resp := GetCredentialResponse{
		ID:         gotCredential.Credential.ID,
		Credential: gotCredential.Credential,
	}
	return framework.Respond(ctx, w, resp, http.StatusOK)
}

type GetCredentialsResponse struct {
	Credentials []credsdk.VerifiableCredential `json:"credentials"`
}

// GetCredentials godoc
// @Summary      Get Credentials
// @Description  Checks for the presence of a query parameter and calls the associated filtered get method
// @Tags         CredentialAPI
// @Accept       json
// @Produce      json
// @Param        issuer   query     string  false  "string issuer"
// @Param        schema   query     string  false  "string schema"
// @Param        subject  query     string  false  "string subject"
// @Success      200      {object}  GetCredentialsResponse
// @Failure      400      {string}  string  "Bad request"
// @Failure      500      {string}  string  "Internal server error"
// @Router       /v1/credentials [get]
func (cr CredentialRouter) GetCredentials(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	issuer := framework.GetQueryValue(r, IssuerParam)
	schema := framework.GetQueryValue(r, SchemaParam)
	subject := framework.GetQueryValue(r, SubjectParam)

	err := framework.NewRequestErrorMsg("must use one of the following query parameters: issuer, subject, schema", http.StatusBadRequest)

	// check if there are multiple parameters set, which is not allowed
	if (issuer != nil && subject != nil) || (issuer != nil && schema != nil) || (subject != nil && schema != nil) {
		return err
	}

	if issuer != nil {
		return cr.getCredentialsByIssuer(*issuer, ctx, w, r)
	}
	if subject != nil {
		return cr.getCredentialsBySubject(*subject, ctx, w, r)
	}
	if schema != nil {
		return cr.getCredentialsBySchema(*schema, ctx, w, r)
	}
	return err
}

func (cr CredentialRouter) getCredentialsByIssuer(issuer string, ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	gotCredentials, err := cr.service.GetCredentialsByIssuer(credential.GetCredentialByIssuerRequest{Issuer: issuer})
	if err != nil {
		errMsg := fmt.Sprintf("could not get credentials for issuer: %s", util.SanitizeLog(issuer))
		logrus.WithError(err).Error(errMsg)
		return framework.NewRequestError(errors.Wrap(err, errMsg), http.StatusInternalServerError)
	}

	resp := GetCredentialsResponse{Credentials: gotCredentials.Credentials}
	return framework.Respond(ctx, w, resp, http.StatusOK)
}

func (cr CredentialRouter) getCredentialsBySubject(subject string, ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	gotCredentials, err := cr.service.GetCredentialsBySubject(credential.GetCredentialBySubjectRequest{Subject: subject})
	if err != nil {
		errMsg := fmt.Sprintf("could not get credentials for subject: %s", util.SanitizeLog(subject))
		logrus.WithError(err).Error(errMsg)
		return framework.NewRequestError(errors.Wrap(err, errMsg), http.StatusInternalServerError)
	}

	resp := GetCredentialsResponse{Credentials: gotCredentials.Credentials}
	return framework.Respond(ctx, w, resp, http.StatusOK)
}

func (cr CredentialRouter) getCredentialsBySchema(schema string, ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	gotCredentials, err := cr.service.GetCredentialsBySchema(credential.GetCredentialBySchemaRequest{Schema: schema})
	if err != nil {
		errMsg := fmt.Sprintf("could not get credentials for schema: %s", util.SanitizeLog(schema))
		logrus.WithError(err).Error(errMsg)
		return framework.NewRequestError(errors.Wrap(err, errMsg), http.StatusInternalServerError)
	}

	resp := GetCredentialsResponse{Credentials: gotCredentials.Credentials}
	return framework.Respond(ctx, w, resp, http.StatusOK)
}

// DeleteCredential godoc
// @Summary      Delete Credentials
// @Description  Delete credential by ID
// @Tags         CredentialAPI
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "ID"
// @Success      200  {string}  string  "OK"
// @Failure      400  {string}  string  "Bad request"
// @Failure      500  {string}  string  "Internal server error"
// @Router       /v1/credentials/{id} [delete]
func (cr CredentialRouter) DeleteCredential(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	id := framework.GetParam(ctx, IDParam)
	if id == nil {
		errMsg := "cannot delete credential without ID parameter"
		logrus.Error(errMsg)
		return framework.NewRequestErrorMsg(errMsg, http.StatusBadRequest)
	}

	if err := cr.service.DeleteCredential(credential.DeleteCredentialRequest{ID: *id}); err != nil {
		errMsg := fmt.Sprintf("could not delete credential with id: %s", *id)
		logrus.WithError(err).Error(errMsg)
		return framework.NewRequestError(errors.Wrap(err, errMsg), http.StatusInternalServerError)
	}

	return framework.Respond(ctx, w, nil, http.StatusOK)
}
