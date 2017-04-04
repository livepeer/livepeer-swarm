package network

import (
	"strings"

	"github.com/livepeer/go-livepeer/livepeer/storage/streaming"
)

type StreamDB struct {
	DownstreamRequesters        map[streaming.StreamID][]*peer
	UpstreamTranscodeRequesters map[streaming.StreamID]*peer
	TranscodedStreams           map[streaming.StreamID][]transcodedStreamData
}

func NewStreamDB() *StreamDB {
	return &StreamDB{
		DownstreamRequesters:        make(map[streaming.StreamID][]*peer),
		UpstreamTranscodeRequesters: make(map[streaming.StreamID]*peer),
		TranscodedStreams:           make(map[streaming.StreamID][]transcodedStreamData),
	}
}

func (self *StreamDB) AddDownstreamPeer(streamID streaming.StreamID, p *peer) {
	self.DownstreamRequesters[streamID] = append(self.DownstreamRequesters[streamID], p)
}

func (self *StreamDB) RemoveDownstreamPeer(streamID streaming.StreamID, p *peer) {
	peers := self.DownstreamRequesters[streamID]
	removei := -1

	for pi, peer := range peers {
		if strings.Compare(p.Addr().String(), peer.Addr().String()) == 0 {
			removei = pi
		}
	}

	if removei > -1 {
		peers[removei] = peers[len(peers)-1]
		self.DownstreamRequesters[streamID] = peers[:len(peers)-1]
	}
}

func (self *StreamDB) AddUpstreamTranscodeRequester(transcodeID streaming.StreamID, p *peer) {
	self.UpstreamTranscodeRequesters[transcodeID] = p
}

func (self *StreamDB) AddTranscodedStream(originalStreamID streaming.StreamID, transcodedStream transcodedStreamData) {
	self.TranscodedStreams[originalStreamID] = append(self.TranscodedStreams[originalStreamID], transcodedStream)
}
