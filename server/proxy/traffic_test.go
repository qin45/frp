// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/fatedier/frp/server/metrics"
)

type fakeServerMetrics struct {
	mu     sync.Mutex
	in     int64
	out    int64
	closed int
}

func (m *fakeServerMetrics) NewClient()                              {}
func (m *fakeServerMetrics) CloseClient()                            {}
func (m *fakeServerMetrics) NewProxy(string, string, string, string) {}
func (m *fakeServerMetrics) CloseProxy(string, string)               {}
func (m *fakeServerMetrics) OpenConnection(string, string)           {}
func (m *fakeServerMetrics) CloseConnection(string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed++
}

func (m *fakeServerMetrics) AddTrafficIn(_ string, _ string, n int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.in += n
}

func (m *fakeServerMetrics) AddTrafficOut(_ string, _ string, n int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.out += n
}

func (m *fakeServerMetrics) snapshot() (in, out int64, closed int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.in, m.out, m.closed
}

func TestTrafficStatsReporterPeriodicFlush(t *testing.T) {
	old := metrics.Server
	fake := &fakeServerMetrics{}
	metrics.Server = fake
	t.Cleanup(func() {
		metrics.Server = old
	})

	reporter := newTrafficStatsReporter("proxy", "tcp", 10*time.Millisecond, func() {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		fake.closed++
	})
	reporter.AddIn(100)
	reporter.AddOut(200)

	deadline := time.Now().Add(time.Second)
	for {
		in, out, _ := fake.snapshot()
		if in == 100 && out == 200 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("traffic was not flushed while connection was active, got in=%d out=%d", in, out)
		}
		time.Sleep(time.Millisecond)
	}

	reporter.AddIn(50)
	reporter.Close()
	reporter.Close()

	in, out, closed := fake.snapshot()
	if in != 150 || out != 200 || closed != 1 {
		t.Fatalf("unexpected final stats: in=%d out=%d closed=%d", in, out, closed)
	}
}

func TestTrafficStatsReporterDisabledInterval(t *testing.T) {
	old := metrics.Server
	fake := &fakeServerMetrics{}
	metrics.Server = fake
	t.Cleanup(func() {
		metrics.Server = old
	})

	reporter := newTrafficStatsReporter("proxy", "tcp", 0, nil)
	reporter.AddIn(10)
	reporter.AddOut(20)
	time.Sleep(20 * time.Millisecond)

	in, out, _ := fake.snapshot()
	if in != 0 || out != 0 {
		t.Fatalf("interval 0 should not flush before close, got in=%d out=%d", in, out)
	}

	reporter.Close()
	in, out, _ = fake.snapshot()
	if in != 10 || out != 20 {
		t.Fatalf("close should flush remaining traffic, got in=%d out=%d", in, out)
	}
}

type fakeReadWriteCloser struct {
	readData []byte
}

func (f *fakeReadWriteCloser) Read(p []byte) (int, error) {
	n := copy(p, f.readData)
	f.readData = f.readData[n:]
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (f *fakeReadWriteCloser) Write(p []byte) (int, error) {
	return len(p), nil
}

func (f *fakeReadWriteCloser) Close() error {
	return nil
}

type fakeNetConn struct {
	readData []byte
}

func (f *fakeNetConn) Read(p []byte) (int, error) {
	n := copy(p, f.readData)
	f.readData = f.readData[n:]
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (f *fakeNetConn) Write(p []byte) (int, error)      { return len(p), nil }
func (f *fakeNetConn) Close() error                     { return nil }
func (f *fakeNetConn) LocalAddr() net.Addr              { return nil }
func (f *fakeNetConn) RemoteAddr() net.Addr             { return nil }
func (f *fakeNetConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeNetConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeNetConn) SetWriteDeadline(time.Time) error { return nil }

func TestTrafficStatsWrapperDirections(t *testing.T) {
	old := metrics.Server
	fake := &fakeServerMetrics{}
	metrics.Server = fake
	t.Cleanup(func() {
		metrics.Server = old
	})

	rwcReporter := newTrafficStatsReporter("proxy", "tcp", 0, nil)
	rwc := &trafficStatsRWC{
		ReadWriteCloser: &fakeReadWriteCloser{readData: []byte("de")},
		reporter:        rwcReporter,
	}
	if _, err := rwc.Write([]byte("abc")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := rwc.Read(buf); err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if err := rwc.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	connReporter := newTrafficStatsReporter("proxy", "tcp", 0, nil)
	conn := &trafficStatsConn{
		Conn:     &fakeNetConn{readData: []byte("xy")},
		reporter: connReporter,
	}
	if _, err := conn.Write([]byte("12345")); err != nil {
		t.Fatalf("conn write failed: %v", err)
	}
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("conn read failed: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("conn close failed: %v", err)
	}

	in, out, _ := fake.snapshot()
	if in != 8 || out != 4 {
		t.Fatalf("wrapper directions mismatch: in=%d out=%d, want in=8 out=4", in, out)
	}
}
