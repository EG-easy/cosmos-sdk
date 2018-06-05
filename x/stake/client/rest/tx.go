package rest

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/tendermint/go-crypto/keys"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"

	"github.com/cosmos/cosmos-sdk/client/context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/wire"
	"github.com/cosmos/cosmos-sdk/x/stake"
)

func registerTxRoutes(ctx context.CoreContext, r *mux.Router, cdc *wire.Codec, kb keys.Keybase) {
	r.HandleFunc(
		"/stake/delegations",
		editDelegationsRequestHandlerFn(cdc, kb, ctx),
	).Methods("POST")
}

// request body for edit delegations
type EditDelegationsBody struct {
	LocalAccountName string                    `json:"name"`
	Password         string                    `json:"password"`
	ChainID          string                    `json:"chain_id"`
	Sequence         int64                     `json:"sequence"`
	Delegations      []stake.MsgDelegate       `json:"delegations"`
	BeginUnbondings  []stake.MsgBeginUnbonding `json:"begin_unbondings"`
}

func editDelegationsRequestHandlerFn(cdc *wire.Codec, kb keys.Keybase, ctx context.CoreContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req EditDelegationsBody
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		err = cdc.UnmarshalJSON(body, &req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		info, err := kb.Get(req.LocalAccountName)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(err.Error()))
			return
		}

		// build messages
		messages := make([]sdk.Msg, len(req.Delegations)+len(req.BeginUnbondings))
		i := 0
		for _, msg := range req.Delegations {
			if !bytes.Equal(info.Address(), msg.DelegatorAddr) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Must use own delegator address"))
				return
			}
			messages[i] = msg
			i++
		}
		for _, msg := range req.BeginUnbondings {
			if !bytes.Equal(info.Address(), msg.DelegatorAddr) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Must use own delegator address"))
				return
			}
			messages[i] = msg
			i++
		}

		// sign messages
		signedTxs := make([][]byte, len(messages[:]))
		for i, msg := range messages {
			// increment sequence for each message
			ctx = ctx.WithSequence(req.Sequence)
			req.Sequence++

			txBytes, err := ctx.SignAndBuild(req.LocalAccountName, req.Password, msg, cdc)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(err.Error()))
				return
			}

			signedTxs[i] = txBytes
		}

		// send
		// XXX the operation might not be atomic if a tx fails
		//     should we have a sdk.MultiMsg type to make sending atomic?
		results := make([]*ctypes.ResultBroadcastTxCommit, len(signedTxs[:]))
		for i, txBytes := range signedTxs {
			res, err := ctx.BroadcastTx(txBytes)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			}
			results[i] = res
		}

		output, err := json.MarshalIndent(results[:], "", "  ")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		w.Write(output)
	}
}
