package soap_test

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

type echoReq struct {
	XMLName xml.Name `xml:"http://tempuri.org/ Echo"`
	Text    string   `xml:"http://tempuri.org/ text"`
}

type echoResp struct {
	XMLName xml.Name `xml:"EchoResponse"`
	Result  string   `xml:"EchoResult"`
}

func TestCallRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("SOAPAction") != `"http://tempuri.org/Echo"` {
			t.Errorf("SOAPAction = %q", r.Header.Get("SOAPAction"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "Security") || !strings.Contains(string(body), ">merhaba<") {
			t.Errorf("request: %s", body)
		}
		_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><EchoResponse xmlns="http://tempuri.org/"><EchoResult>merhaba</EchoResult></EchoResponse></s:Body></s:Envelope>`)
	}))
	defer srv.Close()
	c := &soap.Client{URL: srv.URL, Header: soap.NewUsernameToken("u", "p"), Provider: "t"}
	var out echoResp
	if err := c.Call(context.Background(), "http://tempuri.org/Echo", echoReq{Text: "merhaba"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Result != "merhaba" {
		t.Fatalf("result %q", out.Result)
	}
}

func TestFault(t *testing.T) {
	for name, body := range map[string]string{
		"11": `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>Kullanıcı adı veya şifre hatalı</faultstring></s:Fault></s:Body></s:Envelope>`,
		"12": `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><s:Fault><s:Code><s:Value>s:Sender</s:Value></s:Code><s:Reason><s:Text xml:lang="tr">Authentication failed</s:Text></s:Reason></s:Fault></s:Body></s:Envelope>`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			c := &soap.Client{URL: srv.URL, Provider: "t"}
			if name == "12" {
				c.Version = soap.V12
			}
			err := c.Call(context.Background(), "A", echoReq{}, nil)
			var f *soap.Fault
			if !errors.As(err, &f) || !errors.Is(err, provider.ErrAuth) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// unqualifiedReq has a namespaced root with unqualified children, as
// schemas with elementFormDefault="unqualified" require.
type unqualifiedReq struct {
	XMLName xml.Name `xml:"tns:Req"`
	NS      string   `xml:"xmlns:tns,attr"`
	Child   string   `xml:"CHILD"`
}

func TestMarshalKeepsUnqualifiedChildren(t *testing.T) {
	for _, v := range []soap.Version{soap.V11, soap.V12} {
		c := &soap.Client{Version: v, Header: soap.NewUsernameToken("u", "p")}
		b, err := c.Marshal(unqualifiedReq{NS: "urn:x", Child: "v"})
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(strings.NewReader(string(b)))
		seen := map[string]xml.Name{}
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if se, ok := tok.(xml.StartElement); ok {
				seen[se.Name.Local] = se.Name
			}
		}
		want := map[string]string{"Envelope": envNS(v), "Header": envNS(v), "Body": envNS(v), "Req": "urn:x", "CHILD": ""}
		for local, ns := range want {
			if got, ok := seen[local]; !ok || got.Space != ns {
				t.Errorf("v%d %s namespace = %q, want %q\n%s", v, local, got.Space, ns, b)
			}
		}
	}
}

func envNS(v soap.Version) string {
	if v == soap.V12 {
		return "http://www.w3.org/2003/05/soap-envelope"
	}
	return "http://schemas.xmlsoap.org/soap/envelope/"
}

func TestHTTPErrorWithoutEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "down")
	}))
	defer srv.Close()
	err := (&soap.Client{URL: srv.URL}).Call(context.Background(), "A", echoReq{}, nil)
	if !errors.Is(err, provider.ErrUnavailable) || !provider.IsRetryable(err) {
		t.Fatalf("err = %v", err)
	}
}
