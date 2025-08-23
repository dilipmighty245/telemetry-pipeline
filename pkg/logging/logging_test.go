//go:build unit
// +build unit

package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var testBuffer = new(bytes.Buffer)

func TestMain(m *testing.M) {
	srv := StartDefaultEndpoint()
	defer func() {
		srv.Shutdown(context.Background())
	}()

	setup()
	os.Exit(m.Run())
}

func setup() {
	logInstance.logger = logInstance.logger.WithOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapcore.NewCore(
			zapcore.NewConsoleEncoder(
				zap.NewDevelopmentEncoderConfig()),
			zapcore.AddSync(testBuffer),
			logInstance.atom,
		)
	}))
}

func TestBasicLogs(t *testing.T) {
	testBuffer.Reset()
	table := []struct {
		msg string
		fn  func(msg string, args ...interface{})
	}{{
		msg: "debug",
		fn:  Debugf,
	}, {
		msg: "info",
		fn:  Infof,
	}, {
		msg: "warn",
		fn:  Warnf,
	}, {
		msg: "error",
		fn:  Errorf,
	}}

	for _, entry := range table {
		entry.fn(entry.msg)
		output := testBuffer.String()
		if !strings.Contains(output, entry.msg) {
			t.Fatalf("expected %s", entry.msg)
		}
	}
}

func TestLogsWithParams(t *testing.T) {
	logMsg := "log string"
	logInt := 5
	logFloat := 3.14
	table := []struct {
		log string
		fn  func(string, ...interface{})
	}{{
		log: "zap Debug log, msg %s, int %d, float %0.2f",
		fn:  Debugf,
	}, {
		log: "zap Info log, msg %s, int %d, float %0.2f",
		fn:  Infof,
	}, {
		log: "zap Warn log, msg %s, int %d, float %0.2f",
		fn:  Warnf,
	}, {
		log: "zap Error log, msg %s, int %d, float %0.2f",
		fn:  Errorf,
	}}

	for _, entry := range table {
		testBuffer.Reset()
		entry.fn(entry.log, logMsg, logInt, logFloat)
		expected := fmt.Sprintf(entry.log, logMsg, logInt, logFloat)
		output := testBuffer.String()
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %s", expected)
		}
	}
}

func TestLogsSetLogLevel(t *testing.T) {
	table := []struct {
		log   string
		level string
		files []string
		fn    func(string, ...interface{})
	}{{
		log:   "Debug log",
		level: "Debug",
		files: []string{"", "logging_test"},
		fn:    Debugf,
	}, {
		log:   "Info log",
		level: "Info",
		files: []string{"", "logging_test"},
		fn:    Infof,
	}, {
		log:   "Warn log",
		level: "Warn",
		files: []string{"", "logging_test"},
		fn:    Warnf,
	}, {
		log:   "Error log",
		level: "Error",
		files: []string{"", "logging_test"},
		fn:    Errorf,
	}}

	for idx, entry := range table {
		testBuffer.Reset()
		entry.fn(entry.log)
		output := testBuffer.String()
		if !strings.Contains(output, entry.log) {
			t.Fatalf("expected %s", entry.log)
		}

		for _, file := range entry.files {
			testBuffer.Reset()
			SetLogLevel(entry.level, file)
			entry.fn(entry.log)
			output = testBuffer.String()
			if !strings.Contains(output, entry.log) {
				t.Fatalf("expected %s", entry.log)
			}
		}

		if idx == 0 {
			continue
		}

		i := idx - 1
		for ; i >= 0; i-- {
			testBuffer.Reset()
			table[i].fn(table[i].log)
			if testBuffer.Len() > 0 {
				t.Fatalf("Should not log at %s level", table[i].level)
			}
		}
	}
}

func TestLogsChangeGlobalLogLevel(t *testing.T) {

	// make sure HTTP server is up
	for {
		_, err := http.Get("http://localhost:3000/log")
		if err != nil && strings.Contains(err.Error(), "connection refused") {
			time.Sleep(100 * time.Millisecond)
		} else {
			break
		}
	}
	postBody := []byte("")
	postResp, err := http.Post("http://localhost:3000/log?level=info", "body/type", bytes.NewBuffer(postBody))
	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(postResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var message payload
	err = json.Unmarshal(body, &message)
	if err != nil {
		t.Fatal(err)
	}
	if message.Level != "info" {
		t.Fatal("Error: unexpected error level " + message.Level)
	}

	getResp, err := http.Get("http://localhost:3000/log")
	if err != nil {
		t.Fatal(err)
	}

	body, err = ioutil.ReadAll(getResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(body, &message)
	if err != nil {
		t.Fatal(err)
	}
	if message.Level != "info" {
		t.Fatal("Error: unexpected error level " + message.Level)
	}
}

func TestLogsChangeFileLogLevel(t *testing.T) {
	postBody := []byte("")
	postResp, err := http.Post("http://localhost:3000/log?file=unit_test&level=warn", "body/type", bytes.NewBuffer(postBody))
	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(postResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var message payload
	err = json.Unmarshal(body, &message)
	if err != nil {
		t.Fatal(err)
	}
	if message.Level != "warn" {
		t.Fatal("Error: unexpected error level " + message.Level)
	}

	getResp, err := http.Get("http://localhost:3000/log?file=unit_test")
	if err != nil {
		t.Fatal(err)
	}

	body, err = ioutil.ReadAll(getResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(body, &message)
	if err != nil {
		t.Fatal(err)
	}
	if message.Level != "warn" {
		t.Fatal("Error: unexpected error level " + message.Level)
	}
}

func TestLogsGetLevel(t *testing.T) {
	getResp, err := http.Get("http://localhost:3000/log")
	if err != nil {
		t.Fatal(err)
	}

	if getResp.StatusCode != http.StatusOK {
		t.Fatal("Unexpected Return code " + strconv.Itoa(getResp.StatusCode) + ": " + getResp.Status)
	}

	getResp, err = http.Get("http://localhost:3000/log?file=logging")
	if err != nil {
		t.Fatal(err)
	}

	// Expect failure here since we have no log level set for logging.go.
	if getResp.StatusCode != http.StatusBadRequest {
		t.Fatal("Unexpected Return code " + strconv.Itoa(getResp.StatusCode) + ": " + getResp.Status)
	}

	postBody := []byte("")
	postResp, err := http.Post("http://localhost:3000/log?file=logging&level=info", "body/type", bytes.NewBuffer(postBody))
	if err != nil {
		t.Fatal(err)
	}

	if postResp.StatusCode != http.StatusOK {
		t.Fatal("Failed to update log level: " + getResp.Status)
	}

	getResp, err = http.Get("http://localhost:3000/log?file=logging")
	if err != nil {
		t.Fatal(err)
	}

	// Now expect success.
	if getResp.StatusCode != http.StatusOK {
		t.Fatal("Unexpected Return code " + strconv.Itoa(getResp.StatusCode) + ": " + getResp.Status)
	}
}

func TestLogsBadHttpRequest(t *testing.T) {
	getResp, err := http.Get("http://localhost:3000/log?file=badbadfile")
	if err != nil {
		t.Fatal(err)
	}
	if getResp.StatusCode != http.StatusBadRequest {
		t.Fatal("Unexpected Return code " + strconv.Itoa(getResp.StatusCode) + ": " + getResp.Status)
	}
}
