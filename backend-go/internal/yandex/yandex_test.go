package yandex

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeTransport — возвращает заранее заданный ответ или ошибку.
type fakeTransport struct {
	status int
	body   string
	err    error
	reqs   []string
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.reqs = append(f.reqs, req.Method+" "+req.URL.String()+" auth="+req.Header.Get("Authorization"))
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{
		StatusCode: f.status, Body: io.NopCloser(strings.NewReader(f.body)),
		Header: http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func newClient(tr http.RoundTripper) *http.Client {
	return &http.Client{Transport: tr}
}

var userInfo = `{"status":"ok","rooms":[{"id":"r1","name":"Спальня"}],"devices":[
	{"id":"d1","name":"Лампа цитрусы","room":"r1","type":"devices.types.socket","capabilities":[{"type":"devices.capabilities.on_off"}]},
	{"id":"d2","name":"Датчик","room":null,"type":"devices.types.sensor","capabilities":[]}]}`

func TestListDevicesKeepsSwitchableWithRoom(t *testing.T) {
	tr := &fakeTransport{status: 200, body: userInfo}
	old := client
	client = newClient(tr)
	defer func() { client = old }()

	devs, err := ListDevices("tok")
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].ID != "d1" || devs[0].Name != "Лампа цитрусы" {
		t.Fatalf("devs %+v", devs)
	}
	if devs[0].Room == nil || *devs[0].Room != "Спальня" {
		t.Fatalf("room %v", devs[0].Room)
	}
	if !strings.Contains(tr.reqs[0], "Bearer tok") {
		t.Fatalf("auth %v", tr.reqs[0])
	}
}

func TestSetOnSendsOnOffAction(t *testing.T) {
	tr := &fakeTransport{status: 200, body: `{"status":"ok","devices":[{"id":"d1","capabilities":[
		{"type":"devices.capabilities.on_off","state":{"instance":"on","action_result":{"status":"DONE"}}}]}]}`}
	old := client
	client = newClient(tr)
	defer func() { client = old }()

	if err := SetOn("tok", "d1", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tr.reqs[0], "POST") || !strings.Contains(tr.reqs[0], "/v1.0/devices/actions") {
		t.Fatalf("req %v", tr.reqs[0])
	}
}

func TestDeviceErrorIsRaised(t *testing.T) {
	tr := &fakeTransport{status: 200, body: `{"status":"ok","devices":[{"id":"d1","capabilities":[
		{"type":"devices.capabilities.on_off","state":{"instance":"on","action_result":{"status":"ERROR","error_code":"DEVICE_UNREACHABLE","error_message":"Устройство не в сети"}}}]}]}`}
	old := client
	client = newClient(tr)
	defer func() { client = old }()

	err := SetOn("tok", "d1", true)
	var ye *YandexError
	if !errors.As(err, &ye) || ye.Error() != "Устройство не в сети" {
		t.Fatalf("err %v", err)
	}
}

func TestAuthErrors(t *testing.T) {
	for _, code := range []int{401, 403} {
		tr := &fakeTransport{status: code, body: "{}"}
		old := client
		client = newClient(tr)
		_, err := ListDevices("tok")
		client = old
		var ae *YandexAuthError
		if !errors.As(err, &ae) {
			t.Fatalf("code %d: %v", code, err)
		}
	}
}

func TestNetworkAndServerErrors(t *testing.T) {
	old := client
	client = newClient(&fakeTransport{status: 500, body: "boom"})
	_, err := ListDevices("tok")
	client = old
	var ae *YandexAuthError
	if errors.As(err, &ae) {
		t.Fatalf("server error became auth: %v", err)
	}
	var ye *YandexError
	if !errors.As(err, &ye) {
		t.Fatalf("server error not YandexError: %v", err)
	}

	client = newClient(&fakeTransport{err: fmt.Errorf("timeout")})
	err = SetOn("tok", "d1", false)
	client = old
	if err != ErrYandexUnavailable {
		t.Fatalf("net error %v", err)
	}
}
