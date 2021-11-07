package dcache

// Refer to the coredns cache plugin test
// https://github.com/coredns/coredns/blob/002b748ccd6b7cc2e3a65f1bd71509f80b95d342/plugin/cache/cache_test.go

import (
	"context"
	"testing"
	"time"

	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type cacheTestCase struct {
	test.Case
	in                 test.Case
	AuthenticatedData  bool
	RecursionAvailable bool
	Truncated          bool
	shouldCache        bool
}

var cacheTestCases = []cacheTestCase{
	{
		RecursionAvailable: true, AuthenticatedData: true,
		Case: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("bootjp.me.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("bootjp.me.	3600	IN	MX	10 aspmx2.googlemail.com."),
			},
		},
		in: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("bootjp.me.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("bootjp.me.	3600	IN	MX	10 aspmx2.googlemail.com."),
			},
		},
		shouldCache: true,
	},
	{
		RecursionAvailable: true, AuthenticatedData: true,
		Case: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("bootjp.me.	3600	IN	A	104.21.15.181"),
				test.A("bootjp.me.	3600	IN	A	195.201.182.103"),
			},
		},
		in: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("bootjp.me.	3600	IN	A	104.21.15.181"),
				test.A("bootjp.me.	3600	IN	A	195.201.182.103"),
			},
		},
		shouldCache: true,
	},
	{
		Truncated: true,
		Case: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeMX,
			Answer: []dns.RR{test.MX("bootjp.me.	1800	IN	MX	1 aspmx.l.google.com.")},
		},
		in:          test.Case{},
		shouldCache: false,
	},
	{
		// TODO impl negative cache and shouldCache true
		RecursionAvailable: true,
		Case: test.Case{
			Rcode: dns.RcodeNameError,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{
				test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
			},
		},
		in: test.Case{
			Rcode: dns.RcodeNameError,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{
				test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
			},
		},
		shouldCache: false,
	},
	{
		// todo impl negative cache and shouldCache true
		RecursionAvailable: true,
		Case: test.Case{
			Rcode: dns.RcodeServerFailure,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		in: test.Case{
			Rcode: dns.RcodeServerFailure,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		shouldCache: false,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Rcode: dns.RcodeNotImplemented,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		in: test.Case{
			Rcode: dns.RcodeNotImplemented,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		shouldCache: false,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("bootjp.me.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("bootjp.me.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("bootjp.me.	3600	IN	RRSIG	MX 8 2 1800 20160521031301 20160421031301 12051 bootjp.me. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		in: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("bootjp.me.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("bootjp.me.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("bootjp.me.	3600	IN	RRSIG	MX 8 2 1800 20160521031301 20160421031301 12051 bootjp.me. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		shouldCache: true,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Qname: "example.org.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("example.org.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("example.org.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("example.org.	3600	IN	RRSIG	MX 8 2 1800 20170521031301 20170421031301 12051 bootjp.me. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		in: test.Case{
			Qname: "example.org.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("example.org.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("example.org.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("example.org.	3600	IN	RRSIG	MX 8 2 1800 20170521031301 20170421031301 12051 bootjp.me. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		shouldCache: true,
	},
}

func cacheMsg(m *dns.Msg, tc cacheTestCase) *dns.Msg {
	m.RecursionAvailable = tc.RecursionAvailable
	m.AuthenticatedData = tc.AuthenticatedData
	m.Authoritative = true
	m.Rcode = tc.Rcode
	m.Truncated = tc.Truncated
	m.Answer = tc.in.Answer
	m.Ns = tc.in.Ns
	// m.Extra = tc.in.Extra don't copy Extra, because we don't care and fake EDNS0 DO with tc.Do.
	return m
}

func newTestCache() (*Dcache, dns.ResponseWriter) {
	c := New("127.0.0.1:6379")
	log := clog.P{}
	crr := NewResponsePrinter(nil, log, c)
	return c, crr
}

func TestCache(t *testing.T) {
	c, _ := newTestCache()
	if err := c.connect(); err != nil {
		t.Fatalf("failed connect %s", err)
	}
	go c.run()
	time.Sleep(time.Second)

	for _, tc := range cacheTestCases {
		m := tc.in.Msg()
		m = cacheMsg(m, tc)

		state := &request.Request{W: &test.ResponseWriter{}, Req: m}

		ans := &AnswerCache{
			Response:  m,
			Type:      dns.Type(state.QType()),
			Do:        state.Do(),
			TimeToDie: time.Now().UTC().Add(1 * time.Minute).Unix(),
		}
		c.publish(ans)

		time.Sleep(time.Second)

		res, ok := c.cache.Get(time.Now().UTC().Unix(), state)

		if ok != tc.shouldCache {
			t.Errorf("Cached message that should not have been cached: %s type: %s", state.Name(), state.Type())
			continue
		}

		if ok {
			resp := res.Response

			if err := test.Header(tc.Case, resp); err != nil {
				t.Logf("Cache %v", resp)
				t.Error(err)
				continue
			}

			if err := test.Section(tc.Case, test.Answer, resp.Answer); err != nil {
				t.Logf("Cache %v -- %v", test.Answer, resp.Answer)
				t.Error(err)
			}
			if err := test.Section(tc.Case, test.Ns, resp.Ns); err != nil {
				t.Error(err)
			}
			if err := test.Section(tc.Case, test.Extra, resp.Extra); err != nil {
				t.Error(err)
			}
		}
	}
}

func BenchmarkCacheResponse(b *testing.B) {
	c := New("127.0.0.1:6379")
	c.connect()
	c.Next = BackendHandler()

	ctx := context.TODO()

	reqs := make([]*dns.Msg, 5)
	for i, q := range []string{"example1", "example2", "a", "b", "ddd"} {
		reqs[i] = new(dns.Msg)
		reqs[i].SetQuestion(q+".example.org.", dns.TypeA)
	}

	b.StartTimer()

	j := 0
	for i := 0; i < b.N; i++ {
		req := reqs[j]
		c.ServeDNS(ctx, &test.ResponseWriter{}, req)
		j = (j + 1) % 5
	}
}

//
func BackendHandler() plugin.Handler {
	return plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response = true
		m.RecursionAvailable = true

		owner := m.Question[0].Name
		m.Answer = []dns.RR{test.A(owner + " 303 IN A 127.0.0.53")}

		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	})
}
