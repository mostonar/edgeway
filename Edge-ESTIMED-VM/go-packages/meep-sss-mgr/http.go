package sssmgr

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	"github.com/gorilla/mux"
)

type SssMgrHttp struct {
	notify func(map[string]interface{})
	path   string
}

const (
	headerAccept      = "application/json"
	headerContentType = "application/json"
	notificationPort  = "31122"
)

var (
	notificationMu            sync.Mutex
	notificationServerStarted bool
	notificationByPath        = map[string]func(map[string]interface{}){}
)

func NewSssMgrHttp() (http_mgr *SssMgrHttp) {
	log.Info(">>> NewSssMgrHttp")
	return new(SssMgrHttp)
}

func (http_mgr *SssMgrHttp) init(tm *SssMgr, notify func(map[string]interface{})) (err error) {
	log.Info(">>> http.init: cse_name: ", tm.cse_name, " host_id: ", tm.hostId, " port: ", tm.port)

	http_mgr.notify = notify
	http_mgr.path = "/" + tm.cse_name

	notificationMu.Lock()
	notificationByPath[http_mgr.path] = notify
	startServer := !notificationServerStarted
	notificationServerStarted = true
	notificationMu.Unlock()

	if !startServer {
		log.Info("Init: Notification server already running, registered ", http_mgr.path)
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleNotification)
	go func() {
		log.Info("Init: Starting OneM2M Notification server, port: " + notificationPort)
		err := http.ListenAndServe(":"+notificationPort, mux)
		if err != nil {
			log.Error(err.Error())
			notificationMu.Lock()
			notificationServerStarted = false
			notificationMu.Unlock()
			return
		}

		log.Info("<<< Init: Terminating OneM2M Notification server")
		notificationMu.Lock()
		notificationServerStarted = false
		notificationMu.Unlock()
	}()

	return nil
}

func handleNotification(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> handleRoot: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	notificationMu.Lock()
	notify := notificationByPath[r.URL.Path]
	notificationMu.Unlock()

	// Get URL
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	if r.Method == "POST" { // Notification
		// Extract body
		bodyBytes, _ := ioutil.ReadAll(r.Body)
		log.Info("subscriptionsPost: bodyBytes: ", string(bodyBytes))
		var body map[string]map[string]interface{}
		err := json.Unmarshal(bodyBytes, &body)
		if err != nil {
			log.Error(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err.Error())
			return
		}
		log.Info("handleRoot: body: ", body)
		if _, ok := body["m2m:sgn"]; !ok {
			err := errors.New("Only m2m:sgn is expected")
			log.Error(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, err.Error())
			return
		}
		// Call callback
		m := body["m2m:sgn"]
		log.Info("handleRoot: m: ", m)
		if notify != nil {
			notify(m)
		}
	} else {
		log.Warn("handleRoot: Unsupported method", r.Method)
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (http_mgr *SssMgrHttp) uninit() (err error) {
	log.Info(">>> uninit")

	notificationMu.Lock()
	delete(notificationByPath, http_mgr.path)
	notificationMu.Unlock()

	return nil
}

func (http_mgr *SssMgrHttp) send(p_ctx SssMgrBindingProtocolContext) (err error, resp interface{}) {
	log.Info(">>> send: ", p_ctx)

	// Build the headers
	var headers = http.Header{}
	headers["Accept"] = []string{headerAccept}
	originator := p_ctx.from
	if !strings.HasPrefix(originator, "C") && !strings.HasPrefix(originator, "S") && !strings.HasPrefix(originator, "/") {
		originator = "C" + originator
	}
	headers["X-M2M-ORIGIN"] = []string{originator}
	headers["X-M2M-RI"] = []string{p_ctx.rqi}
	headers["X-M2M-RVI"] = p_ctx.rvi
	s := headerContentType
	if p_ctx.ty != -1 {
		s = s + ";ty=" + strconv.Itoa(p_ctx.ty)
	}
	headers["Content-Type"] = []string{s}
	// Build the URL
	var url string
	to := p_ctx.to
	if to != p_ctx.name && !strings.Contains(to, "/") && p_ctx.name != "" {
		to = p_ctx.name + "/" + to
	}
	// Honor the scheme of the registered endpoint URI; fall back to the port when it is unknown
	scheme := p_ctx.scheme + "://"
	if p_ctx.scheme == "" {
		scheme = "https://"
		if p_ctx.port != 443 {
			scheme = "http://"
		}
	}
	basePath := strings.Trim(p_ctx.path, "/")
	if basePath != "" {
		url = scheme + p_ctx.host + ":" + strconv.Itoa(p_ctx.port) + "/" + basePath + "/" + to
	} else {
		url = scheme + p_ctx.host + ":" + strconv.Itoa(p_ctx.port) + "/" + to
	}
	// Set the method
	method := ""
	if p_ctx.op == 1 {
		method = "POST"
	} else if p_ctx.op == 2 {
		method = "GET"
	} else if p_ctx.op == 3 {
		method = "PATCH"
	} else if p_ctx.op == 4 {
		method = "DELETE"
	} else {
		err := errors.New("Invalid p_ctx.op")
		log.Error("send: ", err.Error())
		return err, nil
	}
	// Finalize the body
	log.Debug("send: url=", url)
	var response []byte
	if p_ctx.body != nil { // With body
		log.Debug("send: p_ctx.body=", p_ctx.body)
		body, err := json.Marshal(p_ctx.body)
		if err != nil {
			log.Error("send: ", err.Error())
			return err, nil
		}
		log.Debug("send: Request body: ", string(body))
		response, err = sendRequest(method, url, headers, bytes.NewBuffer(body), nil, p_ctx.queries, 201)
		if err != nil {
			log.Error("send: ", err.Error())
			return err, nil
		}
	} else { // Without body
		response, err = sendRequest(method, url, headers, nil, nil, p_ctx.queries, p_ctx.code)
		if err != nil {
			log.Error("send: ", err.Error())
			return err, nil
		}
	}
	log.Debug("send: response: ", string(response))
	if len(response) != 0 {
		err = json.Unmarshal(response, &resp)
		if err != nil {
			log.Error("send: ", err.Error())
			return err, nil
		}
		log.Debug("send: response: ", resp)
		log.Debug("send: TypeOf(response): ", reflect.TypeOf(resp))
	}

	return nil, resp
}

func sendRequest(method string, url string, headers http.Header, body io.Reader, vars map[string]string, query map[string]string, code int) ([]byte, error) {
	log.Debug(">>> sendRequest: url: ", url)
	log.Debug(">>> sendRequest: headers: ", headers)

	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = ioutil.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}

	maxRetries := 15
	retryDelay := 100 * time.Millisecond
	maxDelay := 5 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewBuffer(bodyBytes)
		}

		req, err := http.NewRequest(method, url, reqBody)
		if err != nil || req == nil {
			return nil, err
		}
		if vars != nil {
			req = mux.SetURLVars(req, vars)
		}
		if query != nil {
			q := req.URL.Query()
			for k, v := range query {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()
		}
		req.Header = headers
		req.Close = true

		if attempt == 0 {
			log.Debug("sendRequest: req: ", req)
		}

		// Skip TLS verification for development/testing
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Transport: tr}
		rr, err := client.Do(req)

		if err != nil {
			log.Debug("sendRequest: attempt ", attempt+1, " failed with error: ", err)
			time.Sleep(retryDelay)
			retryDelay *= 2
			if retryDelay > maxDelay {
				retryDelay = maxDelay
			}
			continue
		}

		// Check the status code is what we expect.
		if attempt == 0 {
			log.Debug("sendRequest: rr: ", rr)
			log.Debug("sendRequest: rr.Body: ", rr.Body)
		}

		status := rr.StatusCode
		if status == 502 || status == 503 {
			log.Debug("sendRequest: attempt ", attempt+1, " received status ", status)
			rr.Body.Close()
			time.Sleep(retryDelay)
			retryDelay *= 2
			if retryDelay > maxDelay {
				retryDelay = maxDelay
			}
			continue
		}

		if status != code {
			responseData, _ := ioutil.ReadAll(rr.Body)
			rr.Body.Close()
			log.Debug("sendRequest: responseData: ", string(responseData))
			s := fmt.Sprintf("Wrong status code - got %v want %v:\r\n%s", status, code, string(responseData))
			return nil, errors.New(s)
		}

		responseData, err := ioutil.ReadAll(rr.Body)
		rr.Body.Close()
		return responseData, err
	}
	return nil, fmt.Errorf("sendRequest failed after %d attempts", maxRetries)
}
