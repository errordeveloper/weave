package proxy

import (
	"bytes"
	"errors"
	. "github.com/weaveworks/weave/common"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

const (
	RAW_STREAM = "application/vnd.docker.raw-stream"
)

type proxy struct {
	Dial func() (net.Conn, error)
}

func targetNetwork(u *url.URL) string {
	return u.Scheme
}

func targetAddress(u *url.URL) (addr string) {
	switch u.Scheme {
	case "tcp":
		addr = u.Host
	case "unix":
		addr = u.Path
	}
	return
}

func NewProxy(targetUrl string) (*proxy, error) {
	u, err := url.Parse(targetUrl)
	if err != nil {
		return nil, err
	}
	return &proxy{
		Dial: func() (net.Conn, error) {
			return net.Dial(targetNetwork(u), targetAddress(u))
		},
	}, nil
}

func (proxy *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Info.Printf("%s %s", r.Method, r.URL)
	req, err := proxy.InterceptRequest(r)
	if err != nil {
		http.Error(w, "Unable to create proxied request", http.StatusInternalServerError)
		Warning.Print(err)
		return
	}

	conn, err := proxy.Dial()
	if err != nil {
		http.Error(w, "Could not connect to target", http.StatusInternalServerError)
		Warning.Print(err)
		return
	}
	client := httputil.NewClientConn(conn, nil)
	defer client.Close()

	bs, _ := ioutil.ReadAll(req.Body)
	body := bytes.NewReader(bs)
	req.Body = ioutil.NopCloser(body)
	resp, _ := client.Do(req)
	body.Seek(0, 0)
	resp = proxy.InterceptResponse(req, resp)

	hdr := w.Header()
	for k, vs := range resp.Header {
		for _, v := range vs {
			hdr.Add(k, v)
		}
	}
	Debug.Printf("Response from target: %s %v", resp.Status, w.Header())

	if resp.Header.Get("Content-Type") == RAW_STREAM {
		doRawStream(w, resp, client)
	} else if resp.TransferEncoding != nil &&
		resp.TransferEncoding[0] == "chunked" {
		doChunkedResponse(w, resp, client)
	} else {
		w.WriteHeader(resp.StatusCode)
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			Warning.Print(err)
		}
	}
}

// Supplied so that we can use with http.Client as a Transport
func (proxy *proxy) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := &http.Transport{
		Dial: func(string, string) (conn net.Conn, err error) {
			conn, err = proxy.Dial()
			return
		},
	}
	res, err := transport.RoundTrip(req)
	return res, err
}

func doRawStream(w http.ResponseWriter, resp *http.Response, client *httputil.ClientConn) {
	w.WriteHeader(resp.StatusCode)
	down, up, rem, err := hijack(w, client)
	defer down.Close()
	defer up.Close()

	if err != nil {
		Error.Fatal(w, "Unable to hijack connection for raw stream mode", http.StatusInternalServerError)
		return
	}

	end := make(chan bool)

	go func() {
		defer close(end)
		_, err := io.Copy(down, rem)
		_, err = io.Copy(down, up)
		if err != nil {
			Warning.Print(err)
		}
	}()
	go func() {
		_, err := io.Copy(up, down)
		if err != nil {
			Warning.Print(err)
		}
		up.(interface {
			CloseWrite() error
		}).CloseWrite()
	}()
	<-end
}

func doChunkedResponse(w http.ResponseWriter, resp *http.Response, client *httputil.ClientConn) {
	// Because we can't go back to request/response after we
	// hijack the connection, we need to close it and make the
	// client open another.
	w.Header().Add("Connection", "close")
	w.WriteHeader(resp.StatusCode)

	down, up, rem, err := hijack(w, client)
	defer up.Close()
	defer down.Close()
	if err != nil {
		Error.Fatal("Unable to hijack response stream for chunked response", http.StatusInternalServerError)
		return
	}
	chunkreader := httputil.NewChunkedReader(io.MultiReader(rem, up))
	chunkwriter := httputil.NewChunkedWriter(down)
	io.Copy(chunkwriter, chunkreader)
	chunkwriter.Close()
	resp.Trailer.Write(down)
	// a chunked response ends with a CRLF
	down.Write([]byte{13, 10})
}

func hijack(w http.ResponseWriter, client *httputil.ClientConn) (down net.Conn, up net.Conn, rem io.Reader, err error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		err = errors.New("Unable to cast to Hijack")
		return
	}
	down, _, err = hj.Hijack()
	if err != nil {
		return
	}
	up, rem = client.Hijack()
	return
}
