package public

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"decentralized-api/cosmosclient"

	"github.com/labstack/echo/v4"
	"github.com/productscience/inference/x/inference/types"
	"github.com/stretchr/testify/require"
)

type fakeQueryServer struct {
	types.UnimplementedQueryServer
	participantByAddressResp    *types.QueryGetParticipantResponse
	accountByAddressResp        *types.QueryAccountByAddressResponse
	lastParticipantQueryAddress string
	lastAccountQueryAddress     string
}

func (f *fakeQueryServer) Participant(_ context.Context, req *types.QueryGetParticipantRequest) (*types.QueryGetParticipantResponse, error) {
	f.lastParticipantQueryAddress = req.Index
	if f.participantByAddressResp != nil {
		return f.participantByAddressResp, nil
	}
	return &types.QueryGetParticipantResponse{}, nil
}

func (f *fakeQueryServer) AccountByAddress(_ context.Context, req *types.QueryAccountByAddressRequest) (*types.QueryAccountByAddressResponse, error) {
	f.lastAccountQueryAddress = req.Address
	if f.accountByAddressResp != nil {
		return f.accountByAddressResp, nil
	}
	return &types.QueryAccountByAddressResponse{}, nil
}

func TestGetAccountByAddress_HappyPath(t *testing.T) {
	const address = "gonka1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqy2m5g"

	fq := &fakeQueryServer{
		accountByAddressResp: &types.QueryAccountByAddressResponse{
			Pubkey:  "test-pubkey",
			Balance: 0,
			Denom:   "ngonka",
		},
	}
	conn, cleanup := startTestGRPCServer(t, fq)
	defer cleanup()

	mc := &cosmosclient.MockCosmosMessageClient{}
	mc.On("NewInferenceQueryClient").Return(types.NewQueryClient(conn))

	e := echo.New()
	s := &Server{e: e, recorder: mc}

	req := httptest.NewRequest(http.MethodGet, "/v2/accounts/"+address, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("address")
	c.SetParamValues(address)

	require.NoError(t, s.getAccountByAddress(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var dto AccountDto
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dto))
	require.Equal(t, "test-pubkey", dto.Pubkey)
	require.Equal(t, int64(0), dto.Balance)
	require.Equal(t, "ngonka", dto.Denom)
	require.Equal(t, address, fq.lastAccountQueryAddress)

	mc.AssertExpectations(t)
}

func TestGetParticipantByAddress_HappyPath(t *testing.T) {
	const address = "gonka1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqy2m5g"

	fq := &fakeQueryServer{
		participantByAddressResp: &types.QueryGetParticipantResponse{
			Participant: types.Participant{
				Address:      address,
				InferenceUrl: "http://node:8080",
				Status:       types.ParticipantStatus_ACTIVE,
			},
		},
	}
	conn, cleanup := startTestGRPCServer(t, fq)
	defer cleanup()

	mc := &cosmosclient.MockCosmosMessageClient{}
	mc.On("NewInferenceQueryClient").Return(types.NewQueryClient(conn))

	e := echo.New()
	s := &Server{e: e, recorder: mc}

	req := httptest.NewRequest(http.MethodGet, "/v2/participants/"+address, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("address")
	c.SetParamValues(address)

	require.NoError(t, s.getParticipantByAddress(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var response types.QueryGetParticipantResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, address, response.Participant.Address)
	require.Equal(t, "http://node:8080", response.Participant.InferenceUrl)
	require.Equal(t, types.ParticipantStatus_ACTIVE, response.Participant.Status)
	require.Equal(t, address, fq.lastParticipantQueryAddress)

	mc.AssertExpectations(t)
}
