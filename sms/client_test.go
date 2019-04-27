package sms_test

import (
	"encoding/json"
	"errors"
	"github.com/amirhosseinab/go-sms-ir/sms"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGetCreditShouldUseToken(t *testing.T) {
	fakeToken := "fake_token"
	got := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("x-sms-ir-secure-token")
	}))
	defer ts.Close()

	token := createFakeToken(fakeToken)
	c := sms.NewBulkSMSClient(token, ts.URL)
	c.GetCredit()
	if got != fakeToken {
		t.Errorf("expected '%s', got '%s'", fakeToken, got)
	}
}

func TestGetCreditShouldUseCorrespondingURL(t *testing.T) {
	got := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
	}))
	defer ts.Close()

	token := createFakeToken("")
	c := sms.NewBulkSMSClient(token, ts.URL)
	c.GetCredit()
	if strings.ToLower(got) != "/credit" {
		t.Errorf("expected '%s', got '%s'", "/credit", got)
	}
}

func TestGetCreditShouldHasJSONContentTypeHeader(t *testing.T) {
	got := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Content-Type")
	}))
	defer ts.Close()

	token := createFakeToken("")
	c := sms.NewBulkSMSClient(token, ts.URL)
	c.GetCredit()
	if strings.ToLower(got) != "application/json" {
		t.Errorf("expected '%s', got '%s'", "application/json", got)
	}

}

func TestGetCreditReturnValue(t *testing.T) {
	validToken := "by_valid_token"
	invalidToken := "by_invalid_token"

	td := []struct {
		token   string
		credit  int
		error   error
		message string
	}{
		{token: validToken, credit: 1, error: nil, message: "valid token should not return error"},
		{token: invalidToken, credit: 0, error: errors.New("invalid token"), message: "invalid token should return error"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type data struct {
			Credit       int  `json:"Credit"`
			IsSuccessful bool `json:"IsSuccessful"`
		}
		var d data
		if r.Header.Get("x-sms-ir-secure-token") == validToken {
			d = data{Credit: 1, IsSuccessful: true}
		}
		if r.Header.Get("x-sms-ir-secure-token") == invalidToken {
			d = data{Credit: 0, IsSuccessful: false}
		}
		_ = json.NewEncoder(w).Encode(&d)
	}))

	defer ts.Close()

	for _, d := range td {
		t.Run(d.token, func(t *testing.T) {
			c := sms.NewBulkSMSClient(createFakeToken(d.token), ts.URL)
			credit, err := c.GetCredit()
			if credit != d.credit || (err != nil && err.Error() != d.error.Error()) {
				t.Error(d.message)
			}
		})
	}
}

func TestGetTokenShouldHasRequiredBody(t *testing.T) {
	apiKey := "fake_api_key"
	secretKey := "fake_secret_key"
	type data struct {
		UserApiKey string `json:"UserApiKey"`
		SecretKey  string `json:"SecretKey"`
	}
	d := data{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&d)
		defer r.Body.Close()
	}))
	defer ts.Close()

	token := sms.NewToken(sms.Config{
		APIKey:       apiKey,
		SecretKey:    secretKey,
		BaseURL:      ts.URL,
		DisableCache: true,
	})
	_, _ = token.Get()
	if d.SecretKey != secretKey {
		t.Errorf("Expected SecretKey: '%s', got '%s'", secretKey, d.SecretKey)
	}
	if d.UserApiKey != apiKey {
		t.Errorf("Expected UserApiKey: '%s', got '%s'", apiKey, d.UserApiKey)
	}
}

func TestGetTokenShouldUseCorrespondingURL(t *testing.T) {
	got := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
	}))
	defer ts.Close()
	_, _ = sms.NewToken(sms.Config{
		BaseURL: ts.URL,
	}).Get()

	if strings.ToLower(got) != "/token" {
		t.Errorf("Expected URL: '%s', got '%s'", "/token", got)
	}
}

func TestGetTokenShouldReturnTokenFromAPIResponse(t *testing.T) {
	token := "fake_token"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			TokenKey     string `json:"TokenKey"`
			IsSuccessful bool   `json:"IsSuccessful"`
		}{
			TokenKey:     token,
			IsSuccessful: true,
		}
		_ = json.NewEncoder(w).Encode(&data)
	}))
	defer ts.Close()

	tk := sms.NewToken(sms.Config{BaseURL: ts.URL})
	got, _ := tk.Get()
	if got != token {
		t.Errorf("expected token '%s', got '%s'", token, got)
	}
}

func TestGetTokenShouldReturnErrorWhenKeysAreInvalid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			TokenKey     string `json:"TokenKey"`
			IsSuccessful bool   `json:"IsSuccessful"`
		}{
			TokenKey:     "",
			IsSuccessful: false,
		}
		_ = json.NewEncoder(w).Encode(&data)
	}))
	defer ts.Close()
	tk := sms.NewToken(sms.Config{BaseURL: ts.URL, DisableCache: true})
	token, err := tk.Get()
	if token != "" || err == nil {
		t.Errorf("expected empty token and error")
	}
}

func TestGetTokenShouldCacheTokenUntilTimedOut(t *testing.T) {
	times := 1
	tokens := map[int]string{1: "one", 2: "tow"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			TokenKey     string `json:"TokenKey"`
			IsSuccessful bool   `json:"IsSuccessful"`
		}{
			TokenKey:     tokens[times],
			IsSuccessful: true,
		}
		_ = json.NewEncoder(w).Encode(&data)
	}))
	defer ts.Close()

	tk1 := sms.NewToken(sms.Config{BaseURL: ts.URL, DisableCache: false})
	t1, _ := tk1.Get()

	times++

	tk2 := sms.NewToken(sms.Config{BaseURL: ts.URL})
	t2, _ := tk2.Get()

	if t1 != t2 {
		t.Errorf("expected from cache: '%s', got '%s'", t1, t2)
	}
}
func TestGetTokenShouldHandlerRaceCondition(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			TokenKey     string `json:"TokenKey"`
			IsSuccessful bool   `json:"IsSuccessful"`
		}{
			TokenKey:     strconv.FormatInt(time.Now().Unix(), 10),
			IsSuccessful: true,
		}
		_ = json.NewEncoder(w).Encode(&data)
	}))
	defer ts.Close()
	wg := &sync.WaitGroup{}
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			tk := sms.NewToken(sms.Config{BaseURL: ts.URL, DisableCache: true})
			_, _ = tk.Get()
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestBulkSMS_SendVerificationCodeShouldUseAppropriateHeaders(t *testing.T) {
	fakeToken := "fake_token"
	gotToken := ""
	gotContentType := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("x-sms-ir-secure-token")
		gotContentType = r.Header.Get("Content-Type")
	}))
	defer ts.Close()

	token := createFakeToken(fakeToken)
	c := sms.NewBulkSMSClient(token, ts.URL)
	_, _ = c.SendVerificationCode("", "")
	if gotToken != fakeToken {
		t.Errorf("expected '%s', got '%s'", fakeToken, gotToken)
	}
	if gotContentType != "application/json" {
		t.Errorf("expected '%s', got '%s'", "application/json", gotContentType)
	}
}

func TestBulkSMS_SendVerificationCodeShouldUseAppropriateURL(t *testing.T) {
	got := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
	}))
	defer ts.Close()

	token := createFakeToken("")
	c := sms.NewBulkSMSClient(token, ts.URL)
	_, _ = c.SendVerificationCode("", "")
	if strings.ToLower(got) != "/verificationcode" {
		t.Errorf("expected '%s', got '%s'", "/VerificationCode", got)
	}
}

func TestBulkSMS_SendVerificationCodeShouldHasBody(t *testing.T) {
	mobile := "fake_mobile"
	code := "fake_code"
	type data struct {
		MobileNumber string `json:"MobileNumber"`
		Code         string `json:"Code"`
	}

	d := data{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&d)
		defer r.Body.Close()
	}))
	defer ts.Close()

	token := createFakeToken("")
	c := sms.NewBulkSMSClient(token, ts.URL)
	_, _ = c.SendVerificationCode(mobile, code)

	if d.MobileNumber != mobile {
		t.Errorf("Expected Mobile: '%s', got '%s'", mobile, d.MobileNumber)
	}
	if d.Code != code {
		t.Errorf("Expected Code: '%s', got '%s'", code, d.Code)
	}
}

func TestBulkSMS_SendVerificationCodeShouldReturnErrorForFailedRequests(t *testing.T) {
	validMobile := "by_valid_mobile"
	invalidMobile := "by_invalid_mobile"
	validVId := "53160177228"
	td := []struct {
		mobile  string
		vId     string
		error   error
		message string
	}{
		{mobile: validMobile, vId: validVId, error: nil, message: "valid mobile should not return error"},
		{mobile: invalidMobile, vId: "0", error: errors.New("invalid mobile"), message: "invalid mobile should return error"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type data struct {
			VerificationCodeId float64 `json:"VerificationCodeId"`
			IsSuccessful       bool    `json:"IsSuccessful"`
		}
		var d data

		body := struct{ MobileNumber string `json:"MobileNumber"` }{}
		_ = json.NewDecoder(r.Body).Decode(&body)

		if body.MobileNumber == validMobile {
			v, _ := strconv.ParseFloat(validVId, 64)
			d = data{VerificationCodeId: v, IsSuccessful: true}
		}
		if body.MobileNumber == invalidMobile {
			d = data{VerificationCodeId: 0.0, IsSuccessful: false}
		}
		_ = json.NewEncoder(w).Encode(&d)
	}))

	defer ts.Close()

	for _, d := range td {
		t.Run(d.mobile, func(t *testing.T) {
			c := sms.NewBulkSMSClient(createFakeToken("fake_token"), ts.URL)
			vId, err := c.SendVerificationCode(d.mobile, "fake_code")
			if vId != d.vId || (err != nil && err.Error() != d.error.Error()) {
				t.Error(d.message)
			}
		})
	}
}