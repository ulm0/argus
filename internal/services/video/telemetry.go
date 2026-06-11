package video

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"

	"github.com/ulm0/argus/internal/logger"
)

// SeiTelemetry mirrors the TypeScript SeiMetadata interface with identical camelCase JSON tags.
type SeiTelemetry struct {
	Version                  uint32  `json:"version"`
	GearState                uint32  `json:"gearState"`
	FrameSeqNo               uint32  `json:"frameSeqNo"`
	VehicleSpeedMps          float32 `json:"vehicleSpeedMps"`
	AcceleratorPedalPosition float32 `json:"acceleratorPedalPosition"`
	SteeringWheelAngle       float32 `json:"steeringWheelAngle"`
	BlinkerOnLeft            bool    `json:"blinkerOnLeft"`
	BlinkerOnRight           bool    `json:"blinkerOnRight"`
	BrakeApplied             bool    `json:"brakeApplied"`
	AutopilotState           uint32  `json:"autopilotState"`
	LatitudeDeg              float64 `json:"latitudeDeg"`
	LongitudeDeg             float64 `json:"longitudeDeg"`
	HeadingDeg               float64 `json:"headingDeg"`
	LinearAccelerationMps2X  float64 `json:"linearAccelerationMps2X"`
	LinearAccelerationMps2Y  float64 `json:"linearAccelerationMps2Y"`
	LinearAccelerationMps2Z  float64 `json:"linearAccelerationMps2Z"`
}

// TelemetryFrame is one decoded video frame's timestamp plus optional SEI payload.
type TelemetryFrame struct {
	Time float64       `json:"time"`
	Sei  *SeiTelemetry `json:"sei"`
}

// ExtractTelemetry parses Tesla SEI NAL units from an MP4 file and writes JSON.
// Only frames with non-nil SEI are emitted, keeping the response small (~5-15 KB).
func (s *Service) ExtractTelemetry(w http.ResponseWriter, videoPath string) {
	f, err := os.Open(videoPath)
	if err != nil {
		http.Error(w, `{"error":"video not found"}`, http.StatusNotFound)
		return
	}
	defer f.Close()

	frames, err := parseTelemetryFrames(f)
	if err != nil {
		http.Error(w, `{"error":"telemetry extraction failed"}`, http.StatusInternalServerError)
		return
	}

	// Filter: only emit frames that actually contain SEI telemetry.
	out := frames[:0]
	for _, fr := range frames {
		if fr.Sei != nil {
			out = append(out, fr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if err := json.NewEncoder(w).Encode(map[string]any{"frames": out}); err != nil {
		logger.L.WithError(err).Warn("ExtractTelemetry: encode error")
	}
}

// ── MP4 parsing ──────────────────────────────────────────────────────────────

func parseTelemetryFrames(f *os.File) ([]TelemetryFrame, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := fi.Size()

	// First pass: locate top-level boxes without reading their data.
	type boxRef struct{ dataStart, dataSize int64 }
	var moovRef, mdatRef boxRef
	foundMoov, foundMdat := false, false

	for pos := int64(0); pos+8 <= fileSize; {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			break
		}
		var hdr [8]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			break
		}
		size := int64(binary.BigEndian.Uint32(hdr[:4]))
		typ := string(hdr[4:8])
		headerSize := int64(8)

		if size == 1 {
			var ext [8]byte
			if _, err := io.ReadFull(f, ext[:]); err != nil {
				break
			}
			size = int64(binary.BigEndian.Uint64(ext[:]))
			headerSize = 16
		} else if size == 0 {
			size = fileSize - pos
		}
		if size < headerSize {
			break
		}

		switch typ {
		case "moov":
			moovRef = boxRef{pos + headerSize, size - headerSize}
			foundMoov = true
		case "mdat":
			mdatRef = boxRef{pos + headerSize, size - headerSize}
			foundMdat = true
		}
		pos += size
	}

	if !foundMoov || !foundMdat {
		return nil, fmt.Errorf("moov or mdat box not found")
	}

	// Read entire moov into memory (metadata box, typically < 1 MB).
	if moovRef.dataSize > 10<<20 {
		return nil, fmt.Errorf("moov box unexpectedly large: %d bytes", moovRef.dataSize)
	}
	moovData := make([]byte, moovRef.dataSize)
	if _, err := f.Seek(moovRef.dataStart, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(f, moovData); err != nil {
		return nil, err
	}

	_, durations, err := parseMoov(moovData)
	if err != nil {
		return nil, err
	}

	// Stream mdat NAL by NAL — avoids loading the full video into Pi RAM.
	if _, err := f.Seek(mdatRef.dataStart, io.SeekStart); err != nil {
		return nil, err
	}
	return streamMdat(f, mdatRef.dataSize, durations)
}

// parseMoov extracts the video timescale and per-frame durations (in ms) from moov bytes.
func parseMoov(moov []byte) (timescale uint32, durations []float64, err error) {
	trakS, trakE, ok := findBox(moov, 0, len(moov), "trak")
	if !ok {
		return 0, nil, fmt.Errorf("trak not found")
	}
	mdiaS, mdiaE, ok := findBox(moov, trakS, trakE, "mdia")
	if !ok {
		return 0, nil, fmt.Errorf("mdia not found")
	}

	// Timescale from mdhd
	mdhdS, _, ok := findBox(moov, mdiaS, mdiaE, "mdhd")
	if !ok {
		return 0, nil, fmt.Errorf("mdhd not found")
	}
	if mdhdS >= len(moov) {
		return 0, nil, fmt.Errorf("mdhd truncated")
	}
	version := moov[mdhdS]
	if version == 0 {
		if mdhdS+16 > len(moov) {
			return 0, nil, fmt.Errorf("mdhd v0 truncated")
		}
		timescale = binary.BigEndian.Uint32(moov[mdhdS+12:])
	} else {
		if mdhdS+24 > len(moov) {
			return 0, nil, fmt.Errorf("mdhd v1 truncated")
		}
		timescale = binary.BigEndian.Uint32(moov[mdhdS+20:])
	}
	if timescale == 0 {
		return 0, nil, fmt.Errorf("timescale is zero")
	}

	// Frame durations from stts
	minfS, minfE, ok := findBox(moov, mdiaS, mdiaE, "minf")
	if !ok {
		return 0, nil, fmt.Errorf("minf not found")
	}
	stblS, stblE, ok := findBox(moov, minfS, minfE, "stbl")
	if !ok {
		return 0, nil, fmt.Errorf("stbl not found")
	}
	sttsS, _, ok := findBox(moov, stblS, stblE, "stts")
	if !ok {
		return 0, nil, fmt.Errorf("stts not found")
	}
	if sttsS+8 > len(moov) {
		return 0, nil, fmt.Errorf("stts truncated")
	}
	// Cap total frame durations so a crafted stts entry (count up to 2^32-1)
	// can't blow up memory parsing an untrusted file from the mounted USB.
	const maxFrames = 1 << 20
	entryCount := binary.BigEndian.Uint32(moov[sttsS+4:])
	pos := sttsS + 8
	for i := 0; i < int(entryCount) && pos+8 <= len(moov); i++ {
		count := binary.BigEndian.Uint32(moov[pos:])
		delta := binary.BigEndian.Uint32(moov[pos+4:])
		ms := float64(delta) / float64(timescale) * 1000
		for j := 0; j < int(count) && len(durations) < maxFrames; j++ {
			durations = append(durations, ms)
		}
		if len(durations) >= maxFrames {
			break
		}
		pos += 8
	}

	return timescale, durations, nil
}

// findBox searches buf[start:end] for a box named name and returns its data range.
func findBox(buf []byte, start, end int, name string) (dataStart, dataEnd int, ok bool) {
	for pos := start; pos+8 <= end && pos+8 <= len(buf); {
		size := int(binary.BigEndian.Uint32(buf[pos:]))
		typ := string(buf[pos+4 : pos+8])
		headerSize := 8

		if size == 1 {
			if pos+16 > len(buf) {
				break
			}
			high := binary.BigEndian.Uint32(buf[pos+8:])
			low := binary.BigEndian.Uint32(buf[pos+12:])
			size = int(uint64(high)<<32 | uint64(low))
			headerSize = 16
		} else if size == 0 {
			size = end - pos
		}
		if size < headerSize {
			break
		}

		dataS := pos + headerSize
		dataE := pos + size
		if dataE > end {
			dataE = end
		}
		if typ == name {
			return dataS, dataE, true
		}
		pos += size
	}
	return 0, 0, false
}

// streamMdat reads NAL units from the mdat stream and builds TelemetryFrames.
// Uses O(1) memory for individual NAL reads; SEI payloads are typically < 1 KB.
func streamMdat(f *os.File, mdatSize int64, durations []float64) ([]TelemetryFrame, error) {
	var frames []TelemetryFrame
	frameIdx := 0
	timeMs := 0.0
	var pendingSei *SeiTelemetry

	lenBuf := make([]byte, 4)
	bytesLeft := mdatSize

	for bytesLeft >= 5 {
		if _, err := io.ReadFull(f, lenBuf); err != nil {
			break
		}
		nalSize := int64(binary.BigEndian.Uint32(lenBuf))
		bytesLeft -= 4

		if nalSize < 1 || nalSize > bytesLeft {
			break
		}

		// Read NAL type byte first; skip rest unless it is a type we care about.
		typeBuf := make([]byte, 1)
		if _, err := io.ReadFull(f, typeBuf); err != nil {
			break
		}
		nalKind := typeBuf[0] & 0x1f

		switch nalKind {
		case 6: // SEI
			rest := make([]byte, nalSize-1)
			if _, err := io.ReadFull(f, rest); err != nil {
				bytesLeft -= nalSize
				goto nextNal
			}
			pendingSei = decodeTeslaSeiNal(append(typeBuf, rest...))

		case 1, 5: // non-IDR slice or IDR slice → video frame boundary
			if _, err := f.Seek(nalSize-1, io.SeekCurrent); err != nil {
				bytesLeft -= nalSize
				goto nextNal
			}
			dur := 33.333
			if frameIdx < len(durations) {
				dur = durations[frameIdx]
			}
			frames = append(frames, TelemetryFrame{Time: timeMs / 1000, Sei: pendingSei})
			timeMs += dur
			frameIdx++
			pendingSei = nil

		default:
			if _, err := f.Seek(nalSize-1, io.SeekCurrent); err != nil {
				bytesLeft -= nalSize
				goto nextNal
			}
		}
		bytesLeft -= nalSize
		continue

	nextNal:
		break
	}

	return frames, nil
}

// ── Tesla SEI decoding ────────────────────────────────────────────────────────

// decodeTeslaSeiNal extracts the Tesla protobuf payload from an H.264 SEI NAL unit.
// Tesla marks its SEI with a run of 0x42 bytes followed by 0x69.
func decodeTeslaSeiNal(nal []byte) *SeiTelemetry {
	if len(nal) < 4 {
		return nil
	}
	// Scan for marker starting at index 3 (byte 0 is the NAL header).
	i := 3
	for i < len(nal) && nal[i] == 0x42 {
		i++
	}
	if i <= 3 || i >= len(nal) || nal[i] != 0x69 {
		return nil
	}
	end := len(nal) - 1 // strip trailing byte (RBSP stop bit region)
	if i+1 >= end {
		return nil
	}
	payload := stripEmulationPrevention(nal[i+1 : end])
	return decodeSeiProtobuf(payload)
}

// stripEmulationPrevention removes H.264 emulation prevention bytes (0x03 after 2+ zeros).
func stripEmulationPrevention(data []byte) []byte {
	out := make([]byte, 0, len(data))
	zeros := 0
	for _, b := range data {
		if zeros >= 2 && b == 0x03 {
			zeros = 0
			continue
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

// decodeSeiProtobuf decodes the fixed Tesla protobuf schema from raw bytes.
// Field → wire-type mapping mirrors the TypeScript decodeSeiProtobuf function exactly.
func decodeSeiProtobuf(data []byte) *SeiTelemetry {
	msg := &SeiTelemetry{}
	pos := 0

	readVarint := func() (uint64, bool) {
		var result uint64
		var shift uint
		for pos < len(data) {
			b := data[pos]
			pos++
			result |= uint64(b&0x7f) << shift
			if b&0x80 == 0 {
				return result, true
			}
			shift += 7
			if shift > 63 {
				break
			}
		}
		return 0, false
	}

	readFixed32 := func() (float32, bool) {
		if pos+4 > len(data) {
			return 0, false
		}
		bits := binary.LittleEndian.Uint32(data[pos:])
		pos += 4
		return math.Float32frombits(bits), true
	}

	readFixed64 := func() (float64, bool) {
		if pos+8 > len(data) {
			return 0, false
		}
		bits := binary.LittleEndian.Uint64(data[pos:])
		pos += 8
		return math.Float64frombits(bits), true
	}

	for pos < len(data) {
		tag, ok := readVarint()
		if !ok {
			break
		}
		fieldNumber := tag >> 3
		wireType := tag & 0x07

		switch wireType {
		case 0: // varint
			val, ok := readVarint()
			if !ok {
				break
			}
			switch fieldNumber {
			case 1:
				msg.Version = uint32(val)
			case 2:
				msg.GearState = uint32(val)
			case 3:
				msg.FrameSeqNo = uint32(val)
			case 7:
				msg.BlinkerOnLeft = val != 0
			case 8:
				msg.BlinkerOnRight = val != 0
			case 9:
				msg.BrakeApplied = val != 0
			case 10:
				msg.AutopilotState = uint32(val)
			}

		case 5: // fixed32 (float)
			val, ok := readFixed32()
			if !ok {
				break
			}
			switch fieldNumber {
			case 4:
				msg.VehicleSpeedMps = val
			case 5:
				msg.AcceleratorPedalPosition = val
			case 6:
				msg.SteeringWheelAngle = val
			}

		case 1: // fixed64 (double)
			val, ok := readFixed64()
			if !ok {
				break
			}
			switch fieldNumber {
			case 11:
				msg.LatitudeDeg = val
			case 12:
				msg.LongitudeDeg = val
			case 13:
				msg.HeadingDeg = val
			case 14:
				msg.LinearAccelerationMps2X = val
			case 15:
				msg.LinearAccelerationMps2Y = val
			case 16:
				msg.LinearAccelerationMps2Z = val
			}

		case 2: // length-delimited — skip unknown fields
			length, ok := readVarint()
			if !ok {
				break
			}
			pos += int(length)

		default:
			// Unknown wire type; partial decode is fine, return what we have.
			return msg
		}
	}

	return msg
}
