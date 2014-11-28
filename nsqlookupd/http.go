package nsqlookupd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	httpprof "net/http/pprof"
	"sync/atomic"

	"github.com/bitly/nsq/util"
)

type httpServer struct {
	ctx *Context
}

func (s *httpServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	err := s.v1Router(w, req)
	if err == nil {
		return
	}

	err = s.deprecatedRouter(w, req)
	if err == nil {
		return
	}

	err = s.debugRouter(w, req)
	if err != nil {
		s.ctx.nsqlookupd.logf("ERROR: %s", err)
		util.ApiResponse(w, 404, "NOT_FOUND", nil)
	}
}

func (s *httpServer) debugRouter(w http.ResponseWriter, req *http.Request) error {
	switch req.URL.Path {
	case "/debug":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doDebug(req) })
	case "/debug/pprof":
		httpprof.Index(w, req)
	case "/debug/pprof/cmdline":
		httpprof.Cmdline(w, req)
	case "/debug/pprof/symbol":
		httpprof.Symbol(w, req)
	case "/debug/pprof/heap":
		httpprof.Handler("heap").ServeHTTP(w, req)
	case "/debug/pprof/goroutine":
		httpprof.Handler("goroutine").ServeHTTP(w, req)
	case "/debug/pprof/profile":
		httpprof.Profile(w, req)
	case "/debug/pprof/block":
		httpprof.Handler("block").ServeHTTP(w, req)
	case "/debug/pprof/threadcreate":
		httpprof.Handler("threadcreate").ServeHTTP(w, req)
	default:
		return errors.New(fmt.Sprintf("404 %s", req.URL.Path))
	}
	return nil
}

func (s *httpServer) v1Router(w http.ResponseWriter, req *http.Request) error {
	switch req.URL.Path {
	case "/ping":
		s.pingHandler(w, req)

	case "/lookup":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doLookup(req) })
	case "/topics":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doTopics(req) })
	case "/channels":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doChannels(req) })
	case "/nodes":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doNodes(req) })

	case "/topic/create":
		util.V1APIResponseWrapper(w, req, util.POSTRequired(req,
			func() (interface{}, error) { return s.doCreateTopic(req) }))
	case "/topic/delete":
		util.V1APIResponseWrapper(w, req, util.POSTRequired(req,
			func() (interface{}, error) { return s.doDeleteTopic(req) }))
	case "/topic/tombstone":
		util.V1APIResponseWrapper(w, req, util.POSTRequired(req,
			func() (interface{}, error) { return s.doTombstoneTopicProducer(req) }))

	case "/channel/create":
		util.V1APIResponseWrapper(w, req, util.POSTRequired(req,
			func() (interface{}, error) { return s.doCreateChannel(req) }))
	case "/channel/delete":
		util.V1APIResponseWrapper(w, req, util.POSTRequired(req,
			func() (interface{}, error) { return s.doDeleteChannel(req) }))

	default:
		return errors.New(fmt.Sprintf("404 %s", req.URL.Path))
	}
	return nil
}

func (s *httpServer) deprecatedRouter(w http.ResponseWriter, req *http.Request) error {
	switch req.URL.Path {
	case "/info":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doInfo(req) })
	case "/delete_topic":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doDeleteTopic(req) })
	case "/delete_channel":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doDeleteChannel(req) })
	case "/tombstone_topic_producer":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doTombstoneTopicProducer(req) })
	case "/create_topic":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doCreateTopic(req) })
	case "/create_channel":
		util.NegotiateAPIResponseWrapper(w, req,
			func() (interface{}, error) { return s.doCreateChannel(req) })
	default:
		return errors.New(fmt.Sprintf("404 %s", req.URL.Path))
	}
	return nil
}

func (s *httpServer) pingHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Length", "2")
	io.WriteString(w, "OK")
}

func (s *httpServer) doInfo(req *http.Request) (interface{}, error) {
	return struct {
		Version string `json:"version"`
	}{
		Version: util.BINARY_VERSION,
	}, nil
}

func (s *httpServer) doTopics(req *http.Request) (interface{}, error) {
	topics := s.ctx.nsqlookupd.DB.FindRegistrations("topic", "*", "").Keys()
	return map[string]interface{}{
		"topics": topics,
	}, nil
}

func (s *httpServer) doChannels(req *http.Request) (interface{}, error) {
	reqParams, err := util.NewReqParams(req)
	if err != nil {
		return nil, util.HTTPError{400, "INVALID_REQUEST"}
	}

	topicName, err := reqParams.Get("topic")
	if err != nil {
		return nil, util.HTTPError{400, "MISSING_ARG_TOPIC"}
	}

	channels := s.ctx.nsqlookupd.DB.FindRegistrations("channel", topicName, "*").SubKeys()
	return map[string]interface{}{
		"channels": channels,
	}, nil
}

func (s *httpServer) doLookup(req *http.Request) (interface{}, error) {
	reqParams, err := util.NewReqParams(req)
	if err != nil {
		return nil, util.HTTPError{400, "INVALID_REQUEST"}
	}

	topicName, err := reqParams.Get("topic")
	if err != nil {
		return nil, util.HTTPError{400, "MISSING_ARG_TOPIC"}
	}

	registration := s.ctx.nsqlookupd.DB.FindRegistrations("topic", topicName, "")
	if len(registration) == 0 {
		return nil, util.HTTPError{404, "TOPIC_NOT_FOUND"}
	}

	channels := s.ctx.nsqlookupd.DB.FindRegistrations("channel", topicName, "*").SubKeys()
	producers := s.ctx.nsqlookupd.DB.FindProducers("topic", topicName, "")
	producers = producers.FilterByActive(s.ctx.nsqlookupd.opts.InactiveProducerTimeout,
		s.ctx.nsqlookupd.opts.TombstoneLifetime)
	return map[string]interface{}{
		"channels":  channels,
		"producers": producers.PeerInfo(),
	}, nil
}

func (s *httpServer) doCreateTopic(req *http.Request) (interface{}, error) {
	reqParams, err := util.NewReqParams(req)
	if err != nil {
		return nil, util.HTTPError{400, "INVALID_REQUEST"}
	}

	topicName, err := reqParams.Get("topic")
	if err != nil {
		return nil, util.HTTPError{400, "MISSING_ARG_TOPIC"}
	}

	if !util.IsValidTopicName(topicName) {
		return nil, util.HTTPError{400, "INVALID_ARG_TOPIC"}
	}

	s.ctx.nsqlookupd.logf("DB: adding topic(%s)", topicName)
	key := Registration{"topic", topicName, ""}
	s.ctx.nsqlookupd.DB.AddRegistration(key)

	return nil, nil
}

func (s *httpServer) doDeleteTopic(req *http.Request) (interface{}, error) {
	reqParams, err := util.NewReqParams(req)
	if err != nil {
		return nil, util.HTTPError{400, "INVALID_REQUEST"}
	}

	topicName, err := reqParams.Get("topic")
	if err != nil {
		return nil, util.HTTPError{400, "MISSING_ARG_TOPIC"}
	}

	registrations := s.ctx.nsqlookupd.DB.FindRegistrations("channel", topicName, "*")
	for _, registration := range registrations {
		s.ctx.nsqlookupd.logf("DB: removing channel(%s) from topic(%s)", registration.SubKey, topicName)
		s.ctx.nsqlookupd.DB.RemoveRegistration(registration)
	}

	registrations = s.ctx.nsqlookupd.DB.FindRegistrations("topic", topicName, "")
	for _, registration := range registrations {
		s.ctx.nsqlookupd.logf("DB: removing topic(%s)", topicName)
		s.ctx.nsqlookupd.DB.RemoveRegistration(registration)
	}

	return nil, nil
}

func (s *httpServer) doTombstoneTopicProducer(req *http.Request) (interface{}, error) {
	reqParams, err := util.NewReqParams(req)
	if err != nil {
		return nil, util.HTTPError{400, "INVALID_REQUEST"}
	}

	topicName, err := reqParams.Get("topic")
	if err != nil {
		return nil, util.HTTPError{400, "MISSING_ARG_TOPIC"}
	}

	node, err := reqParams.Get("node")
	if err != nil {
		return nil, util.HTTPError{400, "MISSING_ARG_NODE"}
	}

	s.ctx.nsqlookupd.logf("DB: setting tombstone for producer@%s of topic(%s)", node, topicName)
	producers := s.ctx.nsqlookupd.DB.FindProducers("topic", topicName, "")
	for _, p := range producers {
		thisNode := fmt.Sprintf("%s:%d", p.peerInfo.HttpBroadcastAddress, p.peerInfo.HttpPort)
		if thisNode == node {
			p.Tombstone()
		}
	}

	return nil, nil
}

func (s *httpServer) doCreateChannel(req *http.Request) (interface{}, error) {
	reqParams, err := util.NewReqParams(req)
	if err != nil {
		return nil, util.HTTPError{400, "INVALID_REQUEST"}
	}

	topicName, channelName, err := util.GetTopicChannelArgs(reqParams)
	if err != nil {
		return nil, util.HTTPError{400, err.Error()}
	}

	s.ctx.nsqlookupd.logf("DB: adding channel(%s) in topic(%s)", channelName, topicName)
	key := Registration{"channel", topicName, channelName}
	s.ctx.nsqlookupd.DB.AddRegistration(key)

	s.ctx.nsqlookupd.logf("DB: adding topic(%s)", topicName)
	key = Registration{"topic", topicName, ""}
	s.ctx.nsqlookupd.DB.AddRegistration(key)

	return nil, nil
}

func (s *httpServer) doDeleteChannel(req *http.Request) (interface{}, error) {
	reqParams, err := util.NewReqParams(req)
	if err != nil {
		return nil, util.HTTPError{400, "INVALID_REQUEST"}
	}

	topicName, channelName, err := util.GetTopicChannelArgs(reqParams)
	if err != nil {
		return nil, util.HTTPError{400, err.Error()}
	}

	registrations := s.ctx.nsqlookupd.DB.FindRegistrations("channel", topicName, channelName)
	if len(registrations) == 0 {
		return nil, util.HTTPError{404, "CHANNEL_NOT_FOUND"}
	}

	s.ctx.nsqlookupd.logf("DB: removing channel(%s) from topic(%s)", channelName, topicName)
	for _, registration := range registrations {
		s.ctx.nsqlookupd.DB.RemoveRegistration(registration)
	}

	return nil, nil
}

type node struct {
	RemoteAddress        string   `json:"remote_address"`
	Hostname             string   `json:"hostname"`
	HttpBroadcastAddress string   `json:"http_broadcast_address"`
	TcpBroadcastAddress  string   `json:"tcp_broadcast_address"`
	TcpPort              int      `json:"tcp_port"`
	HttpPort             int      `json:"http_port"`
	Version              string   `json:"version"`
	Tombstones           []bool   `json:"tombstones"`
	Topics               []string `json:"topics"`
}

func (s *httpServer) doNodes(req *http.Request) (interface{}, error) {
	// dont filter out tombstoned nodes
	producers := s.ctx.nsqlookupd.DB.FindProducers("client", "", "").FilterByActive(
		s.ctx.nsqlookupd.opts.InactiveProducerTimeout, 0)
	nodes := make([]*node, len(producers))
	for i, p := range producers {
		topics := s.ctx.nsqlookupd.DB.LookupRegistrations(p.peerInfo.id).Filter("topic", "*", "").Keys()

		// for each topic find the producer that matches this peer
		// to add tombstone information
		tombstones := make([]bool, len(topics))
		for j, t := range topics {
			topicProducers := s.ctx.nsqlookupd.DB.FindProducers("topic", t, "")
			for _, tp := range topicProducers {
				if tp.peerInfo == p.peerInfo {
					tombstones[j] = tp.IsTombstoned(s.ctx.nsqlookupd.opts.TombstoneLifetime)
				}
			}
		}

		nodes[i] = &node{
			RemoteAddress:        p.peerInfo.RemoteAddress,
			Hostname:             p.peerInfo.Hostname,
			TcpBroadcastAddress:  p.peerInfo.TcpBroadcastAddress,
			TcpPort:              p.peerInfo.TcpPort,
			HttpBroadcastAddress: p.peerInfo.HttpBroadcastAddress,
			HttpPort:             p.peerInfo.HttpPort,
			Version:              p.peerInfo.Version,
			Tombstones:           tombstones,
			Topics:               topics,
		}
	}

	return map[string]interface{}{
		"producers": nodes,
	}, nil
}

func (s *httpServer) doDebug(req *http.Request) (interface{}, error) {
	s.ctx.nsqlookupd.DB.RLock()
	defer s.ctx.nsqlookupd.DB.RUnlock()

	data := make(map[string][]map[string]interface{})
	for r, producers := range s.ctx.nsqlookupd.DB.registrationMap {
		key := r.Category + ":" + r.Key + ":" + r.SubKey
		for _, p := range producers {
			m := map[string]interface{}{
				"id":                     p.peerInfo.id,
				"hostname":               p.peerInfo.Hostname,
				"tcp_broadcast_address":  p.peerInfo.TcpBroadcastAddress,
				"tcp_port":               p.peerInfo.TcpPort,
				"http_broadcast_address": p.peerInfo.HttpBroadcastAddress,
				"http_port":              p.peerInfo.HttpPort,
				"version":                p.peerInfo.Version,
				"last_update":            atomic.LoadInt64(&p.peerInfo.lastUpdate),
				"tombstoned":             p.tombstoned,
				"tombstoned_at":          p.tombstonedAt.UnixNano(),
			}
			data[key] = append(data[key], m)
		}
	}

	return data, nil
}
