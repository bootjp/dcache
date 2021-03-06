package dcache

import (
	"context"
	"hash/fnv"
	"math"
	"net"
	"time"

	gonanoid "github.com/matoous/go-nanoid"

	"github.com/oleiade/lane"

	"github.com/coredns/coredns/plugin/pkg/cache"

	"github.com/coredns/coredns/plugin/metrics"

	"github.com/goccy/go-json"

	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/go-redis/redis/v8"

	"github.com/coredns/coredns/request"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/miekg/dns"
)

const name = "dcache"

// Dcache is a plugin that distribute shard successCache.
type Dcache struct {
	init bool
	Addr string
	Next plugin.Handler

	log          clog.P
	id           string
	successCache *CacheRepository
	errorCache   *CacheRepository
	subscribeCon *redis.Client
	publishCon   *redis.Client
	queue        *lane.Queue
}

func New(host string) *Dcache {
	s, _ := NewCacheRepository(10000)
	e, _ := NewCacheRepository(10000)

	return &Dcache{
		Addr:         host,
		successCache: s,
		errorCache:   e,
		id:           gonanoid.MustID(10),
		queue:        lane.NewQueue(),
	}
}

// ServeDNS implements the plugin.Handler interface.
func (d *Dcache) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := &request.Request{Req: r, W: w}
	unix := time.Now().UTC().Unix()
	rw := NewResponsePrinter(w, d.log, d, *state)
	s := metrics.WithServer(ctx)

	cr, eHit := d.errorCache.Get(unix, state)
	if eHit {
		d.log.Debug("errorCache hit")
		cacheHits.WithLabelValues(s).Inc()
		cr.Response.SetReply(r)
		_ = w.WriteMsg(cr.Response)
		return dns.RcodeSuccess, nil
	}

	cr, sHit := d.successCache.Get(unix, state)
	if sHit {
		d.log.Debug("successCache hit")
		cacheHits.WithLabelValues(s).Inc()
		cr.Response.SetReply(r)
		_ = w.WriteMsg(cr.Response)
		return dns.RcodeSuccess, nil
	}

	cacheMisses.WithLabelValues(s).Inc()
	return plugin.NextOrFailure(d.Name(), d.Next, ctx, rw, r)
}

func (d *Dcache) connect() error {
	ctx := context.Background()

	d.publishCon = redis.NewClient(&redis.Options{
		Addr:     d.Addr,
		PoolSize: 1,
	})

	d.subscribeCon = redis.NewClient(&redis.Options{
		Addr:     d.Addr,
		PoolSize: 1,
	})

	if cmd := d.subscribeCon.Ping(ctx); cmd.Err() != nil {
		d.log.Error("failed connect redis", cmd.Err())
		return cmd.Err()
	}

	if cmd := d.publishCon.Ping(ctx); cmd.Err() != nil {
		d.log.Error("failed connect redis", cmd.Err())
		return cmd.Err()
	}

	return nil
}

// Name implements the Handler interface.
func (d *Dcache) Name() string {
	return name
}

func (d *Dcache) runSubscribe() {
	d.log.Info("start distribute cache receive routine")
	defer func() {
		_ = d.subscribeCon.Close()
	}()
	ctx := context.Background()

	sub := d.subscribeCon.Subscribe(ctx, d.Name())
	for {
		m, err := sub.ReceiveMessage(ctx)
		if err != nil {
			d.log.Errorf("failed receive %s", err)
			s := metrics.WithServer(ctx)
			redisErr.WithLabelValues(s).Inc()
			time.Sleep(10 * time.Second)
			continue
		}

		d.log.Debug("receive message", m.String())

		ans := &AnswerCache{}
		if err := json.Unmarshal([]byte(m.Payload), ans); err != nil {
			d.log.Errorf("error unmarshal %s got %v", err, ans)
			continue
		}

		if d.id == ans.By {
			d.log.Debug("ignore own cache")
			continue
		}

		if ans.Error {
			if err = d.errorCache.Set(ans); err != nil {
				d.log.Errorf("error cache set failed got %v err %s", m, err)
				d.log.Error(err)
			}
			continue
		}

		if err = d.successCache.Set(ans); err != nil {
			d.log.Errorf("success cache set failed got %v err %s", m, err)
			d.log.Error(err)
			continue
		}
	}
}

func (d *Dcache) minTTL(msg *dns.Msg) uint32 {
	min := uint32(math.MaxUint32)
	for _, ans := range msg.Answer {
		if min > ans.Header().Ttl {
			min = ans.Header().Ttl
		}
	}

	return min
}
func (d *Dcache) publish(ans *AnswerCache) {

	// truncated data not cache.
	if ans.Response.Truncated {
		return
	}

	ctx := context.Background()

	b, err := ans.MarshalJSON()
	if err != nil {
		d.log.Errorf("failed marshal %s %v", err, ans)
		return
	}

	cmd := d.publishCon.Publish(ctx, d.Name(), string(b))
	if cmd.Err() != nil {
		s := metrics.WithServer(ctx)
		redisErr.WithLabelValues(s).Inc()
		d.log.Errorf("error publish err %s", cmd.Err())
		return
	}
}

func (d *Dcache) runPublish() {
	d.log.Info("start distribute cache publish routine")
	defer func() {
		_ = d.publishCon.Close()
	}()
	for {
		item := d.queue.Dequeue()
		if item == nil {
			time.Sleep(1 * time.Second)
			continue
		}

		ans := item.(*AnswerCache)
		d.publish(ans)
	}
}

type ResponseWriter struct {
	dns.ResponseWriter
	log        clog.P
	cache      *Dcache
	state      request.Request
	do         bool
	prefetch   bool
	remoteAddr net.Addr
}

// RemoteAddr implements the dns.ResponseWriter interface.
func (r *ResponseWriter) RemoteAddr() net.Addr {
	if r.remoteAddr != nil {
		return r.remoteAddr
	}
	return r.ResponseWriter.RemoteAddr()
}

// NewResponsePrinter returns ResponseWriter.
func NewResponsePrinter(w dns.ResponseWriter, log clog.P, d *Dcache, state request.Request) *ResponseWriter {
	return &ResponseWriter{
		ResponseWriter: w,
		log:            log,
		cache:          d,
		state:          state,
	}
}

// WriteMsg calls the underlying ResponseWriter's WriteMsg method and prints "example" to standard output.
func (r *ResponseWriter) WriteMsg(res *dns.Msg) error {
	do := false
	now := time.Now().UTC()
	mt, opt := response.Typify(res, now)
	if opt != nil {
		do = opt.Do()
	}

	res.Answer = filterRRSlice(res.Answer, do)
	res.Ns = filterRRSlice(res.Ns, do)
	res.Extra = filterRRSlice(res.Extra, do)

	ans := &AnswerCache{
		Name:      r.state.Name(),
		Type:      dns.Type(res.Question[0].Qtype),
		Do:        do,
		Response:  res,
		TimeToDie: now.Unix() + int64(r.cache.minTTL(res)),
		By:        r.cache.id,
	}

	switch mt {
	case
		response.NoError,
		response.Delegation:
		r.cache.queue.Enqueue(ans)
	case
		response.NameError,
		response.NoData,
		response.ServerError:
		ans.Error = true
		r.cache.queue.Enqueue(ans)
	case response.OtherError:
		// do not cache
	default:
		r.log.Warningf("unknown type %#v", mt)
	}

	return r.ResponseWriter.WriteMsg(res)
}

func NewCacheRepository(size int) (*CacheRepository, error) {
	return &CacheRepository{
		items: cache.New(size),
	}, nil
}

type CacheRepository struct {
	items *cache.Cache
}
type AnswerCache struct {
	Name      string   `json:"name"`
	Response  *dns.Msg `json:"response"`
	Type      dns.Type `json:"type"`
	Do        bool     `json:"do"`
	TimeToDie int64    `json:"time_to_die"`
	By        string   `json:"by"`
	Error     bool
}

func (a *AnswerCache) MarshalJSON() ([]byte, error) {
	if a == nil {
		return nil, nil
	}

	b, err := a.Response.Pack()
	if err != nil {
		return nil, err
	}

	return json.Marshal(&struct {
		Response  []byte
		Type      dns.Type
		Do        bool
		TimeToDie int64
		By        string
		Error     bool
		Name      string
	}{
		Response:  b,
		Type:      a.Type,
		Do:        a.Do,
		TimeToDie: a.TimeToDie,
		By:        a.By,
		Error:     a.Error,
		Name:      a.Name,
	})
}
func (a *AnswerCache) UnmarshalJSON(data []byte) error {
	if a == nil {
		return nil
	}

	ans := &struct {
		Type      dns.Type
		Do        bool
		TimeToDie int64
		Response  []byte
		By        string
		Error     bool
		Name      string
	}{
		Type:      a.Type,
		Do:        a.Do,
		TimeToDie: a.TimeToDie,
		By:        a.By,
		Error:     a.Error,
		Name:      a.Name,
	}

	if err := json.Unmarshal(data, &ans); err != nil {
		return err
	}

	a.Type = ans.Type
	a.Do = ans.Do
	a.TimeToDie = ans.TimeToDie
	a.Response = &dns.Msg{}
	a.By = ans.By
	a.Name = ans.Name
	a.Error = ans.Error
	return a.Response.Unpack(ans.Response)
}

// by https://github.com/coredns/coredns/blob/002b748ccd6b7cc2e3a65f1bd71509f80b95d342/plugin/cache/cache.go#L68-L87
func (*CacheRepository) key(qname string, m *dns.Msg, t uint16) (bool, uint64) {
	// We don't store truncated responses.
	if m.Truncated {
		return false, 0
	}
	// Nor errors or Meta or Update.
	if t == uint16(response.OtherError) || t == uint16(response.Meta) || t == uint16(response.Update) {
		return false, 0
	}

	return true, hash(qname, t)
}

func hash(qname string, qtype uint16) uint64 {
	h := fnv.New64()
	_, err := h.Write([]byte{byte(qtype >> 8)})
	if err != nil {
		return 0
	}
	_, err = h.Write([]byte{byte(qtype)})
	if err != nil {
		return 0
	}
	_, err = h.Write([]byte(qname))
	if err != nil {
		return 0
	}
	return h.Sum64()
}

func (c *CacheRepository) Get(now int64, r *request.Request) (*AnswerCache, bool) {
	ok, key := c.key(r.QName(), r.Req, r.QType())
	if !ok {
		return nil, false
	}
	v, ok := c.items.Get(key)

	if !ok {
		return nil, false
	}

	cn, ok := v.(*AnswerCache)
	if !ok {
		s := metrics.WithServer(context.Background())
		corruptedCache.WithLabelValues(s).Inc()
		return nil, false
	}

	expire := now-cn.TimeToDie > 0
	if expire {
		c.items.Remove(key)
		return nil, false
	}

	return cn, true
}

func (c *CacheRepository) Set(msg *AnswerCache) error {
	qtype := msg.Type
	name := msg.Name

	newExtra := make([]dns.RR, len(msg.Response.Extra))

	j := 0
	for _, e := range msg.Response.Extra {
		if e.Header().Rrtype == dns.TypeOPT {
			continue
		}
		newExtra[j] = e
		j++
	}
	msg.Response.Extra = newExtra[:j]

	ok, key := c.key(name, msg.Response, uint16(qtype))
	if !ok {
		return nil
	}
	_ = c.items.Add(key, msg)
	return nil
}

//https://github.com/coredns/coredns/blob/002b748ccd6b7cc2e3a65f1bd71509f80b95d342/plugin/cache/dnssec.go#L24-L46
func filterRRSlice(rrs []dns.RR, do bool) []dns.RR {
	j := 0
	rs := make([]dns.RR, len(rrs), len(rrs))
	for _, r := range rrs {
		if !do && isDNSSEC(r) {
			continue
		}
		if r.Header().Rrtype == dns.TypeOPT {
			continue
		}

		rs[j] = r
		j++
	}
	return rs[:j]
}

// client explicitly asked for it.
//https://github.com/coredns/coredns/blob/002b748ccd6b7cc2e3a65f1bd71509f80b95d342/plugin/cache/dnssec.go#L5-L22
func isDNSSEC(r dns.RR) bool {
	switch r.Header().Rrtype {
	case dns.TypeNSEC:
		return true
	case dns.TypeNSEC3:
		return true
	case dns.TypeDS:
		return true
	case dns.TypeRRSIG:
		return true
	case dns.TypeSIG:
		return true
	}
	return false
}
