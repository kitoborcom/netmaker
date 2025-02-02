package controllers

import (
	"encoding/json"
	"fmt"
	proLogic "github.com/gravitl/netmaker/pro/logic"
	"net/http"

	"github.com/gorilla/mux"
	controller "github.com/gravitl/netmaker/controllers"
	"github.com/gravitl/netmaker/logger"
	"github.com/gravitl/netmaker/logic"
	"github.com/gravitl/netmaker/models"
	"github.com/gravitl/netmaker/mq"
)

// RelayHandlers - handle Pro Relays
func RelayHandlers(r *mux.Router) {

	r.HandleFunc("/api/nodes/{network}/{nodeid}/createrelay", controller.Authorize(false, true, "user", http.HandlerFunc(createRelay))).Methods(http.MethodPost)
	r.HandleFunc("/api/nodes/{network}/{nodeid}/deleterelay", controller.Authorize(false, true, "user", http.HandlerFunc(deleteRelay))).Methods(http.MethodDelete)
}

// swagger:route POST /api/nodes/{network}/{nodeid}/createrelay nodes createRelay
//
// Create a relay.
//
//			Schemes: https
//
//			Security:
//	  		oauth
//
//			Responses:
//				200: nodeResponse
func createRelay(w http.ResponseWriter, r *http.Request) {
	var relayRequest models.RelayRequest
	var params = mux.Vars(r)
	w.Header().Set("Content-Type", "application/json")
	err := json.NewDecoder(r.Body).Decode(&relayRequest)
	if err != nil {
		logger.Log(0, r.Header.Get("user"), "error decoding request body: ", err.Error())
		logic.ReturnErrorResponse(w, r, logic.FormatError(err, "badrequest"))
		return
	}
	relayRequest.NetID = params["network"]
	relayRequest.NodeID = params["nodeid"]
	_, relayNode, err := proLogic.CreateRelay(relayRequest)
	if err != nil {
		logger.Log(0, r.Header.Get("user"),
			fmt.Sprintf("failed to create relay on node [%s] on network [%s]: %v", relayRequest.NodeID, relayRequest.NetID, err))
		logic.ReturnErrorResponse(w, r, logic.FormatError(err, "internal"))
		return
	}
	go mq.PublishPeerUpdate()
	logger.Log(1, r.Header.Get("user"), "created relay on node", relayRequest.NodeID, "on network", relayRequest.NetID)
	apiNode := relayNode.ConvertToAPINode()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(apiNode)
}

// swagger:route DELETE /api/nodes/{network}/{nodeid}/deleterelay nodes deleteRelay
//
// Remove a relay.
//
//			Schemes: https
//
//			Security:
//	  		oauth
//
//			Responses:
//				200: nodeResponse
func deleteRelay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var params = mux.Vars(r)
	nodeid := params["nodeid"]
	netid := params["network"]
	updateNodes, node, err := proLogic.DeleteRelay(netid, nodeid)
	if err != nil {
		logger.Log(0, r.Header.Get("user"), "error decoding request body: ", err.Error())
		logic.ReturnErrorResponse(w, r, logic.FormatError(err, "badrequest"))
		return
	}
	logger.Log(1, r.Header.Get("user"), "deleted relay server", nodeid, "on network", netid)
	go func() {
		for _, relayedNode := range updateNodes {
			err = mq.NodeUpdate(&relayedNode)
			if err != nil {
				logger.Log(1, "relayed node update ", relayedNode.ID.String(), "on network", relayedNode.Network, ": ", err.Error())

			}
			h, err := logic.GetHost(relayedNode.HostID.String())
			if err == nil {
				if h.OS == models.OS_Types.IoT {
					nodes, err := logic.GetAllNodes()
					if err != nil {
						return
					}
					node.IsRelay = true // for iot update to recognise that it has to delete relay peer
					if err = mq.PublishSingleHostPeerUpdate(h, nodes, &node, nil); err != nil {
						logger.Log(1, "failed to publish peer update to host", h.ID.String(), ": ", err.Error())
					}
				}
			}
		}
		mq.PublishPeerUpdate()
	}()
	logger.Log(1, r.Header.Get("user"), "deleted relay on node", node.ID.String(), "on network", node.Network)
	apiNode := node.ConvertToAPINode()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(apiNode)
}
