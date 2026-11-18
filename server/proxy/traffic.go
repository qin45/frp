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
	"sync/atomic"
	"time"

	"github.com/fatedier/frp/server/metrics"
)

// trafficStatsReporter accumulates traffic bytes for one proxy connection and
// periodically flushes them to the metrics collector. Previously frp only
// reported traffic when the connection closed, which made long-lived
// connections invisible to the dashboard, admin API and Prometheus.
type trafficStatsReporter struct {
	name      string
	proxyType string
	interval  time.Duration

	in  atomic.Int64
	out atomic.Int64

	flushMu sync.Mutex

	timerMu sync.Mutex
	timer   *time.Timer
	closed  bool

	closeOnce sync.Once
	onClose   func()
}

func newTrafficStatsReporter(name, proxyType string, interval time.Duration, onClose func()) *trafficStatsReporter {
	return &trafficStatsReporter{
		name:      name,
		proxyType: proxyType,
		interval:  interval,
		onClose:   onClose,
	}
}

// AddIn records bytes sent from the user to the frpc side.
func (r *trafficStatsReporter) AddIn(n int64) {
	if n <= 0 {
		return
	}
	r.in.Add(n)
	r.scheduleFlush()
}

// AddOut records bytes sent from the frpc side to the user.
func (r *trafficStatsReporter) AddOut(n int64) {
	if n <= 0 {
		return
	}
	r.out.Add(n)
	r.scheduleFlush()
}

// scheduleFlush arms a one-shot timer after the first byte of a flush period,
// so a connection that sends a burst and then goes idle still reports the
// traffic without waiting for more I/O or for the connection to close.
func (r *trafficStatsReporter) scheduleFlush() {
	if r.interval <= 0 {
		return
	}
	r.timerMu.Lock()
	defer r.timerMu.Unlock()
	if r.closed || r.timer != nil {
		return
	}
	r.timer = time.AfterFunc(r.interval, r.onTimer)
}

func (r *trafficStatsReporter) onTimer() {
	r.timerMu.Lock()
	if r.closed {
		r.timerMu.Unlock()
		return
	}
	r.timer = nil
	r.timerMu.Unlock()
	r.flush()
}

func (r *trafficStatsReporter) flush() {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()
	if in := r.in.Swap(0); in > 0 {
		metrics.Server.AddTrafficIn(r.name, r.proxyType, in)
	}
	if out := r.out.Swap(0); out > 0 {
		metrics.Server.AddTrafficOut(r.name, r.proxyType, out)
	}
}

// Close flushes any remaining bytes and invokes onClose exactly once.
func (r *trafficStatsReporter) Close() {
	r.closeOnce.Do(func() {
		r.timerMu.Lock()
		r.closed = true
		if r.timer != nil {
			r.timer.Stop()
			r.timer = nil
		}
		r.timerMu.Unlock()

		r.flush()
		if r.onClose != nil {
			r.onClose()
		}
	})
}

// trafficStatsRWC wraps the frpc-side connection used by BaseProxy. Reads from
// the wrapped connection are download traffic, writes are upload traffic.
type trafficStatsRWC struct {
	io.ReadWriteCloser
	reporter *trafficStatsReporter

	closeOnce sync.Once
	closeErr  error
}

func (c *trafficStatsRWC) Read(p []byte) (int, error) {
	n, err := c.ReadWriteCloser.Read(p)
	c.reporter.AddOut(int64(n))
	return n, err
}

func (c *trafficStatsRWC) Write(p []byte) (int, error) {
	n, err := c.ReadWriteCloser.Write(p)
	c.reporter.AddIn(int64(n))
	return n, err
}

func (c *trafficStatsRWC) Close() error {
	c.closeOnce.Do(func() {
		c.reporter.Close()
		c.closeErr = c.ReadWriteCloser.Close()
	})
	return c.closeErr
}

// trafficStatsConn wraps the frpc-side net.Conn used by the HTTP proxy. Reads
// from the wrapped connection are download traffic, writes are upload traffic.
type trafficStatsConn struct {
	net.Conn
	reporter *trafficStatsReporter

	closeOnce sync.Once
	closeErr  error
}

func (c *trafficStatsConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.reporter.AddOut(int64(n))
	return n, err
}

func (c *trafficStatsConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.reporter.AddIn(int64(n))
	return n, err
}

func (c *trafficStatsConn) Close() error {
	c.closeOnce.Do(func() {
		c.reporter.Close()
		c.closeErr = c.Conn.Close()
	})
	return c.closeErr
}
