/*
 * Copyright (c) 2022  The AdvantEDGE Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	pullPollInterval = time.Second
	// Stop once nothing is downloading and nothing new happened for this long;
	// remaining images belong to pods blocked on something else (e.g. an init
	// container waiting for a dependency), not on a download
	pullIdleTimeout = 20 * time.Second
	pullMaxDuration = 30 * time.Minute
	// Tolerates clock skew between this host and the apiserver
	pullClockSkew = 5 * time.Second
	// Width of the image column in the results table
	pullNameWidth = 40
)

// Kubelet event, e.g.:
//
//	Successfully pulled image "quay.io/prometheus/prometheus:v2.35.0" in 48.581s (49.328s including waiting). Image size: 82872637 bytes.
var (
	pulledRegex  = regexp.MustCompile(`Successfully pulled image "([^"]+)" in ([0-9.hmµns]+).*Image size: ([0-9]+) bytes`)
	presentRegex = regexp.MustCompile(`Container image "([^"]+)" already present`)
	imageRegex   = regexp.MustCompile(`[Ii]mage "([^"]+)"`)
)

type k8sEvent struct {
	Metadata struct {
		UID string `json:"uid"`
	} `json:"metadata"`
	InvolvedObject struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"involvedObject"`
	Reason        string    `json:"reason"`
	Message       string    `json:"message"`
	Count         int       `json:"count"`
	LastTimestamp time.Time `json:"lastTimestamp"`
	EventTime     time.Time `json:"eventTime"`
}

type k8sContainerStatus struct {
	Image string `json:"image"`
	State struct {
		Waiting *struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"waiting"`
		Running    *struct{} `json:"running"`
		Terminated *struct{} `json:"terminated"`
	} `json:"state"`
}

type k8sPod struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		InitContainers []struct {
			Image string `json:"image"`
		} `json:"initContainers"`
		Containers []struct {
			Image string `json:"image"`
		} `json:"containers"`
	} `json:"spec"`
	Status struct {
		InitContainerStatuses []k8sContainerStatus `json:"initContainerStatuses"`
		ContainerStatuses     []k8sContainerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type pullState int

const (
	pullWaiting pullState = iota
	pullInFlight
	pullDone
	pullFailed
)

type pullTracker struct {
	cobraCmd   *cobra.Command
	since      time.Time
	registry   string // meep registry, shown as "local registry"
	startedAt  time.Time
	headerDone bool
	reported   map[string]bool // images already printed as a result row
	rxStart    uint64
	tty        bool
	images     map[string]pullState
	started    map[string]time.Time
	seenEvents map[string]int
	lastChange time.Time
	pulledSize int64
	pulled     int
	failed     int
	netIfaces  []string // physical interfaces of the local node; empty if not a node
	lastRx     uint64
	lastRxAt   time.Time
	rate       float64 // bytes/s, smoothed
}

// WatchImagePulls follows the container image downloads triggered by pods
// created since 'since' (i.e. by the releases just deployed) and reports
// them until every image is pulled or failed.
//
// Completed pulls are reported with size, duration and average speed taken
// from the kubelet events, which works for any cluster without privileges.
// When this host is itself a cluster node, the live download rate of its
// physical network interfaces is shown while pulls are in flight (containerd's
// own byte-level progress is only reachable through its root-owned socket).
func WatchImagePulls(since time.Time, registry string, cobraCmd *cobra.Command) {
	t := &pullTracker{
		cobraCmd:   cobraCmd,
		since:      since.Add(-pullClockSkew),
		registry:   strings.TrimSuffix(registry, "/"),
		startedAt:  time.Now(),
		tty:        isTerminal(os.Stdout),
		images:     map[string]pullState{},
		started:    map[string]time.Time{},
		seenEvents: map[string]int{},
		reported:   map[string]bool{},
		lastChange: time.Now(),
	}
	if t.isLocalNode() {
		t.netIfaces = physicalInterfaces()
		t.lastRx, t.lastRxAt = rxBytes(t.netIfaces), time.Now()
		t.rxStart = t.lastRx
	}

	fmt.Println(FormatHeader("Image downloads") + FormatStep(" (skip with --no-progress)"))
	deadline := time.Now().Add(pullMaxDuration)
	lastPlainStatus := time.Time{}
	for {
		t.poll()
		t.sampleRate()

		inFlight, waiting := t.count(pullInFlight), t.count(pullWaiting)
		if inFlight == 0 && (waiting == 0 || time.Since(t.lastChange) > pullIdleTimeout) {
			break
		}
		if time.Now().After(deadline) {
			t.clearStatus()
			fmt.Println(FormatWarning("Stopped following image downloads after " + pullMaxDuration.String()))
			break
		}

		if t.tty {
			fmt.Print("\r\033[K" + t.status())
		} else if time.Since(lastPlainStatus) >= 10*time.Second && inFlight > 0 {
			fmt.Println(t.status())
			lastPlainStatus = time.Now()
		}
		time.Sleep(pullPollInterval)
	}
	t.clearStatus()
	t.summary()
}

func (t *pullTracker) poll() {
	// Pods first: they define which images this deployment needs
	var pods struct {
		Items []k8sPod `json:"items"`
	}
	if !t.kubectlJSON(&pods, "get", "pods", "-A", "-o", "json") {
		return
	}
	podKeys := map[string]bool{}
	for _, p := range pods.Items {
		if p.Metadata.CreationTimestamp.Before(t.since) {
			continue
		}
		podKeys[p.Metadata.Namespace+"/"+p.Metadata.Name] = true
		for _, c := range p.Spec.InitContainers {
			t.track(c.Image)
		}
		for _, c := range p.Spec.Containers {
			t.track(c.Image)
		}
		// A started container proves its image is present, even if its
		// kubelet event was missed or already garbage-collected
		statuses := append(p.Status.InitContainerStatuses, p.Status.ContainerStatuses...)
		for _, s := range statuses {
			if s.State.Running != nil || s.State.Terminated != nil {
				t.complete(s.Image, pullDone)
			} else if w := s.State.Waiting; w != nil && isPullError(w.Reason) {
				t.fail(s.Image, w.Reason+": "+w.Message)
			}
		}
	}

	var events struct {
		Items []k8sEvent `json:"items"`
	}
	if !t.kubectlJSON(&events, "get", "events", "-A", "--field-selector", "involvedObject.kind=Pod", "-o", "json") {
		return
	}
	sort.Slice(events.Items, func(i, j int) bool { return eventTime(events.Items[i]).Before(eventTime(events.Items[j])) })
	for _, e := range events.Items {
		if !podKeys[e.InvolvedObject.Namespace+"/"+e.InvolvedObject.Name] || eventTime(e).Before(t.since) {
			continue
		}
		if t.seenEvents[e.Metadata.UID] == e.Count && e.Count != 0 {
			continue
		}
		t.seenEvents[e.Metadata.UID] = e.Count
		t.handleEvent(e)
	}
}

func (t *pullTracker) handleEvent(e k8sEvent) {
	switch e.Reason {
	case "Pulling":
		if m := imageRegex.FindStringSubmatch(e.Message); m != nil && t.images[m[1]] == pullWaiting {
			t.images[m[1]] = pullInFlight
			t.started[m[1]] = time.Now()
			t.lastChange = time.Now()
		}
	case "Pulled":
		if m := pulledRegex.FindStringSubmatch(e.Message); m != nil {
			if t.reported[m[1]] {
				return
			}
			t.reported[m[1]] = true
			size, _ := strconv.ParseInt(m[3], 10, 64)
			d, _ := time.ParseDuration(m[2])
			// Kubelet reports the full image size even when most layers were
			// already cached (e.g. re-pulling :latest), so a sub-second pull
			// yields a meaningless rate
			speed := "-"
			if d >= time.Second {
				speed = formatBytes(float64(size)/d.Seconds()) + "/s"
			}
			t.printRow("✓", m[1], formatBytes(float64(size)), formatDuration(d), speed, FormatSuccess)
			t.pulled++
			t.pulledSize += size
			t.complete(m[1], pullDone)
		} else if m := presentRegex.FindStringSubmatch(e.Message); m != nil {
			t.complete(m[1], pullDone)
		}
	case "Failed", "ErrImagePull", "InspectFailed", "ErrImageNeverPull":
		if m := imageRegex.FindStringSubmatch(e.Message); m != nil {
			t.fail(m[1], e.Message)
		}
	}
}

func (t *pullTracker) track(image string) {
	if _, ok := t.images[image]; !ok {
		t.images[image] = pullWaiting
		t.lastChange = time.Now()
	}
}

func (t *pullTracker) complete(image string, state pullState) {
	if s, ok := t.images[image]; ok && s != state {
		t.images[image] = state
		t.lastChange = time.Now()
	}
}

func (t *pullTracker) fail(image string, msg string) {
	if t.reported[image] || t.images[image] == pullDone {
		return
	}
	t.reported[image] = true
	t.printRow("✗", image, "-", "-", "failed", FormatError)
	// gRPC errors carry the useful part after "desc = "
	if i := strings.LastIndex(msg, "desc = "); i >= 0 {
		msg = msg[i+len("desc = "):]
	}
	fmt.Println(FormatError("      " + msg))
	t.failed++
	t.complete(image, pullFailed)
}

func (t *pullTracker) count(state pullState) int {
	n := 0
	for _, s := range t.images {
		if s == state {
			n++
		}
	}
	return n
}

func (t *pullTracker) status() string {
	var names []string
	for img, s := range t.images {
		if s == pullInFlight {
			names = append(names, fmt.Sprintf("%s %ds", shortImage(img), int(time.Since(t.started[img]).Seconds())))
		}
	}
	sort.Strings(names)
	done := t.count(pullDone) + t.count(pullFailed)
	s := fmt.Sprintf("  [%d/%d images]", done, len(t.images))
	if t.netIfaces != nil {
		s += fmt.Sprintf(" ↓ %s/s", formatBytes(t.rate))
	}
	if len(names) > 0 {
		if len(names) > 3 {
			names = append(names[:3], fmt.Sprintf("+%d more", len(names)-3))
		}
		s += "  pulling: " + strings.Join(names, ", ")
	}
	return FormatStep(s)
}

// printRow prints one aligned result line, preceded by the column header the
// first time, so the section stays empty-headed until there's something to show
func (t *pullTracker) printRow(mark, image, size, duration, speed string, format func(string) string) {
	t.clearStatus()
	if !t.headerDone {
		fmt.Println(FormatStep(fmt.Sprintf("    %-*s %9s %7s %11s  %s", pullNameWidth, "IMAGE", "SIZE", "TIME", "SPEED", "SOURCE")))
		t.headerDone = true
	}
	name, source := t.splitImage(image)
	fmt.Println(format(fmt.Sprintf("  %s %-*s %9s %7s %11s  %s", mark, pullNameWidth, truncate(name, pullNameWidth), size, duration, speed, source)))
}

// splitImage separates an image reference into a readable name and its
// source registry, e.g. "quay.io/thanos/thanos:v0.41.0" -> "thanos/thanos:v0.41.0", "quay.io"
func (t *pullTracker) splitImage(image string) (name string, source string) {
	if t.registry != "" && strings.HasPrefix(image, t.registry+"/") {
		return strings.TrimPrefix(image, t.registry+"/"), "local registry"
	}
	source, name = "docker.io", image
	if i := strings.Index(image, "/"); i >= 0 {
		host := image[:i]
		if strings.ContainsAny(host, ".:") || host == "localhost" {
			source, name = host, image[i+1:]
		}
	}
	if source == "docker.io" {
		name = strings.TrimPrefix(name, "library/")
	}
	return name, source
}

func (t *pullTracker) clearStatus() {
	if t.tty {
		fmt.Print("\r\033[K")
	}
}

func (t *pullTracker) summary() {
	if len(t.images) == 0 {
		fmt.Println(FormatSuccess("No new pods; no images to download"))
		return
	}
	// Anything done without a download was already on the node, whether a
	// kubelet event or a started container told us
	cached := t.count(pullDone) - t.pulled
	msg := fmt.Sprintf("%d images: %d pulled (%s), %d already present", len(t.images), t.pulled, formatBytes(float64(t.pulledSize)), cached)
	if t.failed > 0 {
		msg += fmt.Sprintf(", %d failed", t.failed)
	}
	if t.netIfaces != nil {
		// Image sizes overstate traffic when layers were already cached;
		// this is what actually came in from outside the node
		msg += " · " + formatBytes(float64(rxBytes(t.netIfaces)-t.rxStart)) + " received"
	}
	msg += " · done in " + formatDuration(time.Since(t.startedAt))
	if t.failed > 0 {
		fmt.Println(FormatError(msg))
	} else {
		fmt.Println(FormatSuccess(msg))
	}
	if waiting := t.count(pullWaiting); waiting > 0 {
		fmt.Println(FormatWarning(fmt.Sprintf("%d image(s) not requested yet (pods still waiting on init containers or scheduling)", waiting)))
	}
}

func (t *pullTracker) sampleRate() {
	if t.netIfaces == nil {
		return
	}
	rx, now := rxBytes(t.netIfaces), time.Now()
	if dt := now.Sub(t.lastRxAt).Seconds(); dt > 0 && rx >= t.lastRx {
		inst := float64(rx-t.lastRx) / dt
		t.rate = 0.5*t.rate + 0.5*inst // smooth out per-second jitter
	}
	t.lastRx, t.lastRxAt = rx, now
}

func (t *pullTracker) kubectlJSON(v interface{}, args ...string) bool {
	out, err := exec.Command("kubectl", args...).Output()
	if err != nil {
		if verbose, _ := t.cobraCmd.Flags().GetBool("verbose"); verbose {
			fmt.Println("kubectl", strings.Join(args, " "), "failed:", err)
		}
		return false
	}
	return json.Unmarshal(out, v) == nil
}

// isLocalNode reports whether this host is one of the cluster nodes, so that
// its network interfaces carry the image download traffic
func (t *pullTracker) isLocalNode() bool {
	out, err := exec.Command("kubectl", "get", "nodes", "-o",
		`jsonpath={range .items[*].status.addresses[?(@.type=="InternalIP")]}{.address}{"\n"}{end}`).Output()
	if err != nil {
		return false
	}
	nodeIPs := map[string]bool{}
	for _, ip := range strings.Fields(string(out)) {
		nodeIPs[ip] = true
	}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && nodeIPs[ipnet.IP.String()] {
			return true
		}
	}
	return false
}

// physicalInterfaces lists interfaces carrying external traffic, excluding
// loopback and the virtual devices of docker/CNI plugins (whose traffic is
// in-cluster, or already counted on the physical interface)
func physicalInterfaces() []string {
	virtual := []string{"lo", "docker", "veth", "cali", "tunl", "vxlan", "flannel", "cni", "br-", "kube-", "virbr", "weave", "cilium", "lxc", "tap", "tun"}
	var ifaces []string
	list, _ := net.Interfaces()
	for _, i := range list {
		isVirtual := false
		for _, p := range virtual {
			if strings.HasPrefix(i.Name, p) {
				isVirtual = true
				break
			}
		}
		if !isVirtual && i.Flags&net.FlagUp != 0 {
			ifaces = append(ifaces, i.Name)
		}
	}
	return ifaces
}

// rxBytes sums received bytes for the given interfaces from /proc/net/dev
func rxBytes(ifaces []string) uint64 {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	wanted := map[string]bool{}
	for _, i := range ifaces {
		wanted[i] = true
	}
	var total uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 2)
		if len(parts) != 2 || !wanted[strings.TrimSpace(parts[0])] {
			continue
		}
		if fields := strings.Fields(parts[1]); len(fields) > 0 {
			n, _ := strconv.ParseUint(fields[0], 10, 64)
			total += n
		}
	}
	return total
}

func isPullError(reason string) bool {
	switch reason {
	case "ErrImagePull", "ImagePullBackOff", "InvalidImageName", "ErrImageNeverPull", "RegistryUnavailable":
		return true
	}
	return false
}

func eventTime(e k8sEvent) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp
	}
	return e.EventTime
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func truncate(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}

func shortImage(image string) string {
	if i := strings.LastIndex(image, "/"); i >= 0 {
		return image[i+1:]
	}
	return image
}

func formatBytes(b float64) string {
	units := []string{"B", "KB", "MB", "GB"}
	i := 0
	for b >= 1000 && i < len(units)-1 {
		b /= 1000
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", b, units[i])
	}
	return fmt.Sprintf("%.1f %s", b, units[i])
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
