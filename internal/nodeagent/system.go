package nodeagent

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// SystemSnapshot is the host telemetry the agent reports with each heartbeat.
type SystemSnapshot struct {
	Hostname      string
	OS            string
	Arch          string
	Kernel        string
	CPUCount      int
	CPUModel      string
	TotalRAMBytes uint64
	UsedRAMBytes  uint64
	CPUUsage      float64
	LoadAvg1      float64
}

// cpuSampler turns the cumulative jiffy counters in /proc/stat into a usage
// percentage by differencing consecutive reads.
type cpuSampler struct {
	mu        sync.Mutex
	prevIdle  uint64
	prevTotal uint64
	hasPrev   bool
}

var sampler = &cpuSampler{}

func Snapshot() SystemSnapshot {
	s := SystemSnapshot{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUCount: runtime.NumCPU(),
	}
	s.Hostname, _ = os.Hostname()
	s.Kernel = readFirstLine("/proc/sys/kernel/osrelease")
	s.CPUModel = cpuModel()
	s.TotalRAMBytes, s.UsedRAMBytes = memory()
	s.CPUUsage = sampler.usage()
	s.LoadAvg1 = loadAvg1()
	return s
}

func readFirstLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		return strings.TrimSpace(sc.Text())
	}
	return ""
}

func cpuModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		// x86 reports "model name"; arm64 reports "Model" or nothing useful.
		if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Model") {
			if idx := strings.Index(line, ":"); idx >= 0 {
				return strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return ""
}

// memory reads /proc/meminfo and reports total and in-use bytes, treating
// MemAvailable as the accurate measure of what applications can still get.
func memory() (total, used uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	var available uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		value *= 1024
		switch fields[0] {
		case "MemTotal:":
			total = value
		case "MemAvailable:":
			available = value
		}
	}
	if total > available {
		used = total - available
	}
	return total, used
}

func loadAvg1() float64 {
	fields := strings.Fields(readFirstLine("/proc/loadavg"))
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[0], 64)
	return v
}

func (c *cpuSampler) usage() float64 {
	line := readFirstLine("/proc/stat")
	if !strings.HasPrefix(line, "cpu ") {
		return 0
	}
	fields := strings.Fields(line)[1:]
	var total, idle uint64
	for i, f := range fields {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			continue
		}
		total += v
		// Fields 3 and 4 are idle and iowait.
		if i == 3 || i == 4 {
			idle += v
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	prevIdle, prevTotal, hasPrev := c.prevIdle, c.prevTotal, c.hasPrev
	c.prevIdle, c.prevTotal, c.hasPrev = idle, total, true
	if !hasPrev || total <= prevTotal {
		return 0
	}
	deltaTotal := float64(total - prevTotal)
	deltaIdle := float64(idle - prevIdle)
	usage := (deltaTotal - deltaIdle) / deltaTotal * 100
	if usage < 0 {
		return 0
	}
	if usage > 100 {
		return 100
	}
	return usage
}
