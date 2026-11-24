# FireflyFrp modifications

This repository is a fork of [fatedier/frp](https://github.com/fatedier/frp)
maintained for FireflyFrp (萤火内网穿透). It is not the upstream frp project.

## Source

- Upstream: <https://github.com/fatedier/frp>
- Fork: <https://github.com/qin45/frp>
- Branch: `feat/live-traffic-kick`
- Commit: `e0f615a77558392adb6d5e01b2228196f20688c8`
- Base merge: `bafec21a`
- frp version: `0.72.0`

## Changes

### Live traffic accounting for active connections

- Added `trafficStatsUpdateIntervalSeconds` (default `0`, preserving upstream
  close-time accounting).
- Added `server/proxy/traffic.go` and `server/proxy/traffic_test.go` to flush
  counters for active connections on a timer, so long-lived connections are
  visible in the Dashboard, Admin API, and Prometheus metrics while running.
- Updated `server/proxy/proxy.go` and `server/proxy/http.go` to wrap TCP/UDP/
  HTTP proxies with the traffic reporter. SUDP keeps close-time accounting.
- Updated `conf/frps_full_example.toml` and the server config/validation files.

### Shared per-user bandwidth limiting

- Added `pkg/util/limit/user_limiter.go` and tests: every tunnel of the same
  user shares one `golang.org/x/time/rate.Limiter`.
- `server/proxy/proxy.go` uses the shared limiter when a proxy carries a user.
- `server/controller/resource.go`, `server/service.go`, and
  `server/api_router.go` register and expose `UserLimiters`.

### Admin API additions

- `POST /api/v2/clients/{key}/kick`: kick one client by run ID.
- `POST /api/v2/users/{user}/kick`: kick all clients of a user.
- `PUT /api/v2/users/{user}/bandwidth?bytesPerSecond=...`: update the shared
  per-user bandwidth limit at runtime.
- `server/http/model/v2.go`: user responses now include `todayTrafficIn`,
  `todayTrafficOut`, and `curConns`; added kick/bandwidth response models.
- `server/control.go`, `server/http/controller.go`, and
  `server/http/controller_v2.go` implement the corresponding control paths.

These APIs are used by the FireflyFrp user service for quota enforcement,
per-user bandwidth, and forced disconnect.

## Build

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -tags "frps,noweb" -ldflags "-s -w" -o frps-linux-amd64 ./cmd/frps

# Windows amd64
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -tags "frps,noweb" -ldflags "-s -w" -o frps-windows-amd64.exe ./cmd/frps
```

## License

frp is licensed under the Apache License, Version 2.0. This fork and its
modifications are distributed under the same license. See [LICENSE](LICENSE).
Copyright of the original frp source remains with the frp Authors.
