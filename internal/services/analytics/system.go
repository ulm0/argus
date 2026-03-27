package analytics

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type CPUMetrics struct {
	UsagePercent float64  `json:"usage_percent"`
	Cores        int      `json:"cores"`
	CapacityMHz  *float64 `json:"capacity_mhz,omitempty"`
	TempC        *float64 `json:"temp_c,omitempty"`
}

type RAMMetrics struct {
	UsedBytes  int64 `json:"used_bytes"`
	TotalBytes int64 `json:"total_bytes"`
}

type PowerMetrics struct {
	Watts *float64 `json:"watts,omitempty"`
}

type SystemMetrics struct {
	CPU   CPUMetrics   `json:"cpu"`
	RAM   RAMMetrics   `json:"ram"`
	Power PowerMetrics `json:"power"`
}

func (s *Service) GetSystemMetrics() SystemMetrics {
	usage := readCPUUsagePercent()
	capMHz := readCPUCapacityMHz()
	tempC := readCPUTempC()
	used, total := readRAMUsage()
	powerW := readPowerWatts()

	return SystemMetrics{
		CPU: CPUMetrics{
			UsagePercent: usage,
			Cores:        runtime.NumCPU(),
			CapacityMHz:  capMHz,
			TempC:        tempC,
		},
		RAM: RAMMetrics{
			UsedBytes:  used,
			TotalBytes: total,
		},
		Power: PowerMetrics{
			Watts: powerW,
		},
	}
}

type cpuTimes struct {
	idle  uint64
	total uint64
}

func readCPUUsagePercent() float64 {
	t1, err := readCPUTimes()
	if err != nil {
		return 0
	}
	time.Sleep(200 * time.Millisecond)
	t2, err := readCPUTimes()
	if err != nil {
		return 0
	}

	totalDelta := float64(t2.total - t1.total)
	idleDelta := float64(t2.idle - t1.idle)
	if totalDelta <= 0 {
		return 0
	}
	return (1 - idleDelta/totalDelta) * 100
}

func readCPUTimes() (cpuTimes, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			return cpuTimes{}, fmt.Errorf("unexpected /proc/stat format")
		}
		var vals []uint64
		for _, f := range fields[1:] {
			v, convErr := strconv.ParseUint(f, 10, 64)
			if convErr != nil {
				return cpuTimes{}, convErr
			}
			vals = append(vals, v)
		}
		var total uint64
		for _, v := range vals {
			total += v
		}
		idle := vals[3]
		if len(vals) > 4 {
			idle += vals[4] // iowait
		}
		return cpuTimes{idle: idle, total: total}, nil
	}

	if err := sc.Err(); err != nil {
		return cpuTimes{}, err
	}
	return cpuTimes{}, fmt.Errorf("cpu line not found")
}

func readCPUCapacityMHz() *float64 {
	for _, p := range []string{
		"/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq",
		"/sys/devices/system/cpu/cpu0/cpufreq/scaling_max_freq",
	} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		khz, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err != nil || khz <= 0 {
			continue
		}
		mhz := khz / 1000.0
		return &mhz
	}
	return nil
}

func readCPUTempC() *float64 {
	b, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return nil
	}
	milli, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	if err != nil || milli <= 0 {
		return nil
	}
	c := milli / 1000.0
	return &c
}

func readRAMUsage() (usedBytes int64, totalBytes int64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	var memTotalKB int64
	var memAvailKB int64

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			memTotalKB = parseMemInfoKB(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			memAvailKB = parseMemInfoKB(line)
		}
	}
	if memTotalKB <= 0 {
		return 0, 0
	}
	if memAvailKB < 0 {
		memAvailKB = 0
	}
	totalBytes = memTotalKB * 1024
	usedBytes = (memTotalKB - memAvailKB) * 1024
	if usedBytes < 0 {
		usedBytes = 0
	}
	return usedBytes, totalBytes
}

func parseMemInfoKB(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func readPowerWatts() *float64 {
	entries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		base := filepath.Join("/sys/class/power_supply", e.Name())
		if watts := readPowerNowWatts(base); watts != nil {
			return watts
		}
	}
	return nil
}

func readPowerNowWatts(base string) *float64 {
	if b, err := os.ReadFile(filepath.Join(base, "power_now")); err == nil {
		microW, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err == nil && microW > 0 {
			w := microW / 1_000_000.0
			return &w
		}
	}

	ib, ierr := os.ReadFile(filepath.Join(base, "current_now"))
	vb, verr := os.ReadFile(filepath.Join(base, "voltage_now"))
	if ierr != nil || verr != nil {
		return nil
	}

	microA, errI := strconv.ParseFloat(strings.TrimSpace(string(ib)), 64)
	microV, errV := strconv.ParseFloat(strings.TrimSpace(string(vb)), 64)
	if errI != nil || errV != nil || microA <= 0 || microV <= 0 {
		return nil
	}
	w := (microA * microV) / 1_000_000_000_000.0
	return &w
}
