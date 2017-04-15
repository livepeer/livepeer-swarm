// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"fmt"
	"math/rand"
	"runtime/debug"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"

	"github.com/ethereum/go-ethereum/swarm/network/kademlia"
	"github.com/livepeer/go-livepeer/livepeer/storage"
	"github.com/livepeer/go-livepeer/livepeer/streaming"
	lpmsStream "github.com/livepeer/lpms/stream"
)

const requesterCount = 3

/*
forwarder implements the CloudStore interface (use by storage.NetStore)
and serves as the cloud store backend orchestrating storage/retrieval/delivery
via the native bzz protocol
which uses an MSB logarithmic distance-based semi-permanent Kademlia table for
* recursive forwarding style routing for retrieval
* smart syncronisation
*/

type forwarder struct {
	hive *Hive
}

func NewForwarder(hive *Hive) *forwarder {
	return &forwarder{hive: hive}
}

// generate a unique id uint64
func generateId() uint64 {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return uint64(r.Int63())
}

var searchTimeout = 3 * time.Second

// forwarding logic
// logic propagating retrieve requests to peers given by the kademlia hive
func (self *forwarder) Retrieve(chunk *storage.Chunk) {
	peers := self.hive.getPeers(chunk.Key, 0)
	glog.V(logger.Detail).Infof("forwarder.Retrieve: %v - received %d peers from KΛÐΞMLIΛ...", chunk.Key.Log(), len(peers))
OUT:
	for _, p := range peers {
		glog.V(logger.Detail).Infof("forwarder.Retrieve: sending retrieveRequest %v to peer [%v]", chunk.Key.Log(), p)
		for _, recipients := range chunk.Req.Requesters {
			for _, recipient := range recipients {
				req := recipient.(*retrieveRequestMsgData)
				if req.from.Addr() == p.Addr() {
					continue OUT
				}
			}
		}
		req := &retrieveRequestMsgData{
			Key: chunk.Key,
			Id:  generateId(),
		}
		var err error
		if p.swap != nil {
			err = p.swap.Add(-1)
		}
		if err == nil {
			p.retrieve(req)
			break OUT
		}
		glog.V(logger.Warn).Infof("forwarder.Retrieve: unable to send retrieveRequest to peer [%v]: %v", chunk.Key.Log(), err)
	}
}

// requests to specific peers given by the kademlia hive
// except for peers that the store request came from (if any)
// delivery queueing taken care of by syncer
func (self *forwarder) Store(chunk *storage.Chunk) {
	var n int
	msg := &storeRequestMsgData{
		Key:   chunk.Key,
		SData: chunk.SData,
	}
	var source *peer
	if chunk.Source != nil {
		source = chunk.Source.(*peer)
	}
	for _, p := range self.hive.getPeers(chunk.Key, 0) {
		glog.V(logger.Detail).Infof("forwarder.Store: %v %v", p, chunk)

		if p.syncer != nil && (source == nil || p.Addr() != source.Addr()) {
			n++
			Deliver(p, msg, PropagateReq)
		}
	}
	glog.V(logger.Detail).Infof("forwarder.Store: sent to %v peers (chunk = %v)", n, chunk)
}

// Stream request - this is to request for a stream, not to do broadcast.  The chunks should arrive in protocol.go
func (self *forwarder) Stream(id string, peerAddr kademlia.Address, format lpmsStream.VideoFormat) {
	s := streaming.StreamID(id)
	nodeID, streamID := s.SplitComponents()
	msg := &streamRequestMsgData{
		OriginNode: nodeID,
		StreamID:   streamID,
		Format:     format,
		Id:         streaming.RequestStreamMsgID,
	}

	key := nodeID.Bytes()

	peers := self.hive.getPeers(key, 2)
	var p *peer

	if len(peers) > 1 {
		if peers[0].Addr() == peerAddr {
			p = peers[1]
		} else {
			p = peers[0]
		}
	} else if len(peers) == 1 {
		p = peers[0]
	} else {
		glog.V(logger.Error).Infof("ERROR: Stream Request Sent To %d Peers\n", len(peers))
		debug.PrintStack()
		return
	}

	p.stream(msg)

}

// Stop stream request - this is to stop the stream after local player is closed.  peerAddr is the original streamer's addr.
func (self *forwarder) StopStream(id string, peerAddr kademlia.Address, format lpmsStream.VideoFormat) {
	s := streaming.StreamID(id)
	nodeID, streamID := s.SplitComponents()
	msg := &stopStreamRequestMsgData{
		OriginNode: nodeID,
		StreamID:   streamID,
		Id:         streaming.StopStreamMsgID,
		Format:     format,
	}

	key := nodeID.Bytes()

	peers := self.hive.getPeers(key, 2)
	var p *peer

	if len(peers) > 1 {
		if peers[0].Addr() == peerAddr {
			p = peers[1]
		} else {
			p = peers[0]
		}
	} else if len(peers) == 1 {
		p = peers[0]
	} else {
		fmt.Println("ERROR: Stream Request Sent To %d Peers\n", len(peers))
		return
	}

	p.stopStream(msg)
}

// Transcode request - this is to request for a node to become a transcoder.  The node should send an Ack to confirm.
func (self *forwarder) Transcode(streamId string, transcodeId common.Hash, formats []string, bitrates []string, codecin string, codeout []string) {
	fmt.Println("Forwarding Transcode Request")
	s := streaming.StreamID(streamId)
	nodeID, streamID := s.SplitComponents()
	msg := &transcodeRequestMsgData{
		OriginNode:     nodeID,
		OriginStreamID: streamID,
		TranscodeID:    transcodeId,
		Id:             streaming.TranscodeRequestMsgID,
		Formats:        formats,
		Bitrates:       bitrates,
		CodecIn:        codecin,
		CodecOut:       codeout,
	}

	glog.V(logger.Info).Infof("In forwarding func, getting peer with transcodeId: %x", transcodeId)
	//We always try to branch out at least 1 node, so that the requested node can NEVER be the transcoding node
	peers := self.hive.getPeers(transcodeId.Bytes(), 1)
	if len(peers) > 0 {
		for _, p := range peers {
			fmt.Println("Sending transcode req to peer: %v", p.Addr())
			p.transcode(msg)
		}
	} else {
		fmt.Errorf("Error: no peer found to forward Transcode request.")
		return
	}
}

// once a chunk is found deliver it to its requesters unless timed out
func (self *forwarder) Deliver(chunk *storage.Chunk) {
	// iterate over request entries
	for id, requesters := range chunk.Req.Requesters {
		counter := requesterCount
		msg := &storeRequestMsgData{
			Key:   chunk.Key,
			SData: chunk.SData,
		}
		var n int
		var req *retrieveRequestMsgData
		// iterate over requesters with the same id
		for id, r := range requesters {
			req = r.(*retrieveRequestMsgData)
			if req.timeout == nil || req.timeout.After(time.Now()) {
				glog.V(logger.Detail).Infof("forwarder.Deliver: %v -> %v", req.Id, req.from)
				msg.Id = uint64(id)
				Deliver(req.from, msg, DeliverReq)
				n++
				counter--
				if counter <= 0 {
					break
				}
			}
		}
		glog.V(logger.Detail).Infof("forwarder.Deliver: submit chunk %v (request id %v) for delivery to %v peers", chunk.Key.Log(), id, n)
	}
}

// initiate delivery of a chunk to a particular peer via syncer#addRequest
// depending on syncer mode and priority settings and sync request type
// this either goes via confirmation roundtrip or queued or pushed directly
func Deliver(p *peer, req interface{}, ty int) {
	p.syncer.addRequest(req, ty)
}

// push chunk over to peer
func Push(p *peer, key storage.Key, priority uint) {
	p.syncer.doDelivery(key, priority, p.syncer.quit)
}
