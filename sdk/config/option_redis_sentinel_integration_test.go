//go:build integration

// Integration tests for the Redis Sentinel support in option_redis.go.
//
// These spin up a real master + replica + 3 sentinels via testcontainers and
// exercise (1) the FailoverClient branch of newClient() and (2) an actual
// failover — neither of which miniredis (used by the unit tests) can represent.
//
// Run (Linux CI):
//
//	go test -tags=integration -v -run 'TestSentinel|TestEnsureRedisClient_Sentinel' -timeout 240s ./sdk/config/
//
// Requires Docker. LINUX CI ONLY — on Docker Desktop (Windows/Mac) the host can
// reach containers only via localhost-published ports, while a sentinel reports
// its monitored master by an in-network address the host cannot route to; the
// sentinel-behind-NAT wall blocks a host-side FailoverClient there. On Linux the
// container/bridge addresses ARE host-routable, so this runs green on Linux CI.
// On Docker Desktop dev machines, rely on the unit coverage of the FailoverClient
// branch wiring: TestRedisConnectOptions_FailoverOptions + TestNewClient_Sentinel_ReturnsFailoverClient.
//
// Topology notes (the host/container addressing is the fiddly part):
//   - All 5 containers share ONE testcontainers network, so sentinels gossip
//     with each other by container IP:26379 and reach quorum without
//     announce-port tricks.
//   - Each container also publishes its internal port on a RANDOM host port, so
//     the host-based go-redis client reaches sentinels via localhost:<mapped>.
//   - Sentinels monitor the master via a HOST-routable address
//     (host.docker.internal:<master-published-port>, else the host LAN IP), so
//     the address a sentinel reports as "master" is resolvable by the host
//     client too. The replica announces the same host address (via CONFIG SET
//     after start, once its published port is known), so the post-failover
//     master is host-reachable as well.
package config

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	tc "github.com/testcontainers/testcontainers-go"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	itImage      = "redis:7-alpine"
	itPwd        = "testpw"
	itMasterName = "mymaster"
)

// hostEndpoint returns an address reachable from BOTH the containers and the
// host (where go-redis runs). Prefer host.docker.internal (Docker Desktop
// resolves it on the host and it's injected into containers via the
// host-gateway extra host); otherwise fall back to the host's first
// non-loopback IPv4, which the containers reach via the Docker bridge.
func hostEndpoint(t *testing.T) string {
	t.Helper()
	if a, err := net.LookupHost("host.docker.internal"); err == nil && len(a) > 0 {
		return "host.docker.internal"
	}
	ifs, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces: %v", err)
	}
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipn.IP.To4(); ip4 != nil && !ip4.IsLinkLocalUnicast() {
				return ip4.String()
			}
		}
	}
	t.Fatal("cannot determine a host endpoint reachable from containers")
	return ""
}

// startOn runs a redis:7-alpine container on the shared network with a RANDOM
// published host port for internalPort. Returns the container; use hostPort()
// to read the mapped port after start.
func startOn(t *testing.T, ctx context.Context, name, internalPort, netName string, cmd []string) tc.Container {
	t.Helper()
	c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: tc.ContainerRequest{
			Image:        itImage,
			Cmd:          cmd,
			ExposedPorts: []string{internalPort + "/tcp"},
			Networks:     []string{netName},
			ExtraHosts:   []string{"host.docker.internal:host-gateway"},
			WaitingFor:   wait.ForListeningPort(internalPort + "/tcp").WithStartupTimeout(40 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	return c
}

func hostPort(t *testing.T, ctx context.Context, c tc.Container, internal string) string {
	t.Helper()
	p, err := c.MappedPort(ctx, internal+"/tcp")
	if err != nil {
		t.Fatalf("mapped port %s: %v", internal, err)
	}
	return p.Port()
}

// sentinelConfig builds a sentinel.conf body. Redis config has no inline
// comments; every directive is on its own line.
func sentinelConfig(host, masterPort string) string {
	return strings.Join([]string{
		"port 26379",
		"protected-mode no",
		"sentinel resolve-hostnames yes",
		"sentinel monitor " + itMasterName + " " + host + " " + masterPort + " 2",
		"sentinel auth-pass " + itMasterName + " " + itPwd,
		"sentinel down-after-milliseconds " + itMasterName + " 3000",
		"sentinel failover-timeout " + itMasterName + " 30000",
		"sentinel parallel-syncs " + itMasterName + " 1",
	}, "\n") + "\n"
}

// waitForTopology polls a sentinel until it reports the master with >=1 replica
// and >=2 peer sentinels — the readiness gate before EnsureRedisClient runs.
func waitForTopology(t *testing.T, sentinelAddr string) {
	t.Helper()
	cl := redis.NewClient(&redis.Options{Addr: sentinelAddr, DialTimeout: 2 * time.Second})
	defer cl.Close()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
		res, err := cl.Do(ctx, "SENTINEL", "master", itMasterName).Result()
		cancel()
		if err == nil {
			m := parseSentinelKVs(res)
			if toInt(m["num-slaves"]) >= 1 && toInt(m["num-other-sentinels"]) >= 2 {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("sentinel topology did not form in time (sentinel=%s)", sentinelAddr)
}

func parseSentinelKVs(res interface{}) map[string]string {
	m := map[string]string{}
	arr, ok := res.([]interface{})
	if !ok {
		return m
	}
	for i := 0; i+1 < len(arr); i += 2 {
		k, _ := arr[i].(string)
		v, _ := arr[i+1].(string)
		m[k] = v
	}
	return m
}

func toInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// setupSentinelTopology brings up master + replica + 3 sentinels. Returns the
// sentinel addresses the host go-redis client should use, the master container
// (so a test can stop it to trigger failover), and a teardown.
func setupSentinelTopology(t *testing.T) (sentinelAddrs []string, master tc.Container, teardown func()) {
	t.Helper()
	ctx := context.Background()
	host := hostEndpoint(t)

	nw, err := tcnetwork.New(ctx)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	netName := nw.Name

	master = startOn(t, ctx, "master", "6379", netName,
		[]string{"redis-server", "--requirepass", itPwd, "--appendonly", "yes", "--masterauth", itPwd})
	pMaster := hostPort(t, ctx, master, "6379")

	replica := startOn(t, ctx, "replica", "6379", netName,
		[]string{"redis-server", "--replicaof", host, pMaster, "--masterauth", itPwd, "--requirepass", itPwd})
	pReplica := hostPort(t, ctx, replica, "6379")
	// Announce a host-routable address so that after failover (replica promoted)
	// the sentinel reports an address the host client can reach.
	rcl := redis.NewClient(&redis.Options{Addr: "localhost:" + pReplica, Password: itPwd, DialTimeout: 2 * time.Second})
	if err := rcl.ConfigSet(ctx, "replica-announce-ip", host).Err(); err != nil {
		t.Fatalf("config set replica-announce-ip: %v", err)
	}
	if err := rcl.ConfigSet(ctx, "replica-announce-port", pReplica).Err(); err != nil {
		t.Fatalf("config set replica-announce-port: %v", err)
	}
	rcl.Close()

	conf := sentinelConfig(host, pMaster)
	var sentinels []tc.Container
	for i := 0; i < 3; i++ {
		script := fmt.Sprintf("cat > /s.conf <<'SENTINEL_EOF'\n%sSENTINEL_EOF\nexec redis-sentinel /s.conf\n", conf)
		s := startOn(t, ctx, fmt.Sprintf("sentinel-%d", i+1), "26379", netName, []string{"sh", "-c", script})
		sentinels = append(sentinels, s)
		sentinelAddrs = append(sentinelAddrs, "localhost:"+hostPort(t, ctx, s, "26379"))
	}

	waitForTopology(t, sentinelAddrs[0])

	teardown = func() {
		tctx, cancel := context.WithTimeout(context.Background(), 30 * time.Second)
		defer cancel()
		for _, s := range sentinels {
			_ = s.Terminate(tctx)
		}
		_ = replica.Terminate(tctx)
		_ = master.Terminate(tctx)
		_ = nw.Remove(tctx)
	}
	return sentinelAddrs, master, teardown
}

// TestEnsureRedisClient_Sentinel verifies the FailoverClient branch of
// newClient(): EnsureRedisClient connects THROUGH sentinels and can round-trip
// SET/GET against the master. (miniredis cannot exercise this branch.)
func TestEnsureRedisClient_Sentinel(t *testing.T) {
	ResetRedisClientsForTest()
	addrs, _, teardown := setupSentinelTopology(t)
	defer teardown()
	defer ResetRedisClientsForTest()

	client, err := EnsureRedisClient(RedisConnectOptions{
		MasterName:    itMasterName,
		SentinelAddrs: addrs,
		Password:      itPwd,
		DB:            0,
	})
	if err != nil {
		t.Fatalf("EnsureRedisClient via sentinel: %v", err)
	}

	ctx := context.Background()
	if err := client.Set(ctx, "sentinel-it", "ok", 0).Err(); err != nil {
		t.Fatalf("SET via FailoverClient: %v", err)
	}
	v, err := client.Get(ctx, "sentinel-it").Result()
	if err != nil || v != "ok" {
		t.Fatalf("GET via FailoverClient = %q, err=%v, want ok", v, err)
	}
}

// TestSentinel_Failover is the core-value assertion: after writing a key, kill
// the master; the go-redis FailoverClient must follow sentinel's promotion of
// the replica and keep reading the key (proving failover happened, the client
// followed without restart, and pre-failover data survived on the replica).
func TestSentinel_Failover(t *testing.T) {
	ResetRedisClientsForTest()
	addrs, master, teardown := setupSentinelTopology(t)
	defer teardown()
	defer ResetRedisClientsForTest()

	client, err := EnsureRedisClient(RedisConnectOptions{
		MasterName:    itMasterName,
		SentinelAddrs: addrs,
		Password:      itPwd,
		DB:            0,
	})
	if err != nil {
		t.Fatalf("EnsureRedisClient: %v", err)
	}

	ctx := context.Background()
	if err := client.Set(ctx, "ha-key", "survived", 0).Err(); err != nil {
		t.Fatalf("SET ha-key: %v", err)
	}
	// Give replication a moment to land on the replica before we kill the master.
	time.Sleep(500 * time.Millisecond)

	// Kill the master -> sentinel elects the replica (down-after=3s, quorum=2).
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10 * time.Second)
	defer stopCancel()
	grace := 0 * time.Second
	if err := master.Stop(stopCtx, &grace); err != nil {
		t.Fatalf("stop master: %v", err)
	}

	// Poll: the SAME client must read ha-key after the failover window. This
	// simultaneously proves promotion, go-redis auto-follow, and data survival.
	deadline := time.Now().Add(60 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		if v, gerr := client.Get(ctx, "ha-key").Result(); gerr == nil && v == "survived" {
			got = v
			break
		}
		time.Sleep(time.Second)
	}
	if got != "survived" {
		t.Fatalf("after master kill, ha-key not readable within failover window — client did not follow failover")
	}
}
