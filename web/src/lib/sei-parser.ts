// Tesla Dashcam MP4 SEI Parser
// Parses MP4 files and extracts SEI telemetry metadata embedded by Tesla in dashcam H.264 streams.

export enum GearState {
  PARK = 0,
  DRIVE = 1,
  REVERSE = 2,
  NEUTRAL = 3,
}

export enum AutopilotState {
  NONE = 0,
  SELF_DRIVING = 1,
  AUTOSTEER = 2,
  TACC = 3,
}

export interface SeiMetadata {
  version: number;
  gearState: GearState;
  frameSeqNo: number;
  vehicleSpeedMps: number;
  acceleratorPedalPosition: number;
  steeringWheelAngle: number;
  blinkerOnLeft: boolean;
  blinkerOnRight: boolean;
  brakeApplied: boolean;
  autopilotState: AutopilotState;
  latitudeDeg: number;
  longitudeDeg: number;
  headingDeg: number;
  linearAccelerationX: number;
  linearAccelerationY: number;
  linearAccelerationZ: number;
}

export interface SeiFrame {
  time: number;
  sei: SeiMetadata | null;
}

interface BoxLocation {
  start: number;
  end: number;
  size: number;
}

interface VideoConfig {
  timescale: number;
  durations: number[];
}

// ── Minimal protobuf decoder for fixed SeiMetadata schema ──

function decodeSeiProtobuf(data: Uint8Array): SeiMetadata | null {
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  const len = data.length;
  let pos = 0;

  const msg: SeiMetadata = {
    version: 0,
    gearState: GearState.PARK,
    frameSeqNo: 0,
    vehicleSpeedMps: 0,
    acceleratorPedalPosition: 0,
    steeringWheelAngle: 0,
    blinkerOnLeft: false,
    blinkerOnRight: false,
    brakeApplied: false,
    autopilotState: AutopilotState.NONE,
    latitudeDeg: 0,
    longitudeDeg: 0,
    headingDeg: 0,
    linearAccelerationX: 0,
    linearAccelerationY: 0,
    linearAccelerationZ: 0,
  };

  function readVarint(): number {
    let result = 0;
    let shift = 0;
    while (pos < len) {
      const b = data[pos++];
      result |= (b & 0x7f) << shift;
      if ((b & 0x80) === 0) return result >>> 0;
      shift += 7;
      if (shift > 35) break;
    }
    return result >>> 0;
  }

  function readFixed32(): number {
    if (pos + 4 > len) { pos = len; return 0; }
    const v = view.getFloat32(pos, true);
    pos += 4;
    return v;
  }

  function readFixed64(): number {
    if (pos + 8 > len) { pos = len; return 0; }
    const v = view.getFloat64(pos, true);
    pos += 8;
    return v;
  }

  try {
    while (pos < len) {
      const tag = readVarint();
      const fieldNumber = tag >>> 3;
      const wireType = tag & 0x07;

      switch (wireType) {
        case 0: {
          const value = readVarint();
          switch (fieldNumber) {
            case 1: msg.version = value; break;
            case 2: msg.gearState = value as GearState; break;
            case 3: msg.frameSeqNo = value; break;
            case 7: msg.blinkerOnLeft = value !== 0; break;
            case 8: msg.blinkerOnRight = value !== 0; break;
            case 9: msg.brakeApplied = value !== 0; break;
            case 10: msg.autopilotState = value as AutopilotState; break;
          }
          break;
        }
        case 5: {
          const value = readFixed32();
          switch (fieldNumber) {
            case 4: msg.vehicleSpeedMps = value; break;
            case 5: msg.acceleratorPedalPosition = value; break;
            case 6: msg.steeringWheelAngle = value; break;
          }
          break;
        }
        case 1: {
          const value = readFixed64();
          switch (fieldNumber) {
            case 11: msg.latitudeDeg = value; break;
            case 12: msg.longitudeDeg = value; break;
            case 13: msg.headingDeg = value; break;
            case 14: msg.linearAccelerationX = value; break;
            case 15: msg.linearAccelerationY = value; break;
            case 16: msg.linearAccelerationZ = value; break;
          }
          break;
        }
        case 2: {
          const length = readVarint();
          pos += length;
          break;
        }
        default:
          return msg;
      }
    }
  } catch {
    // Partial decode is fine -- return what we have
  }

  return msg;
}

// ── H.264 emulation prevention byte removal ──

function stripEmulationBytes(data: Uint8Array): Uint8Array {
  const out: number[] = [];
  let zeros = 0;
  for (const byte of data) {
    if (zeros >= 2 && byte === 0x03) {
      zeros = 0;
      continue;
    }
    out.push(byte);
    zeros = byte === 0 ? zeros + 1 : 0;
  }
  return Uint8Array.from(out);
}

// ── SEI NAL unit decoder ──

function decodeSeiNal(nal: Uint8Array): SeiMetadata | null {
  if (nal.length < 4) return null;

  // Scan for Tesla's marker: sequence of 0x42 bytes followed by 0x69
  let i = 3;
  while (i < nal.length && nal[i] === 0x42) i++;
  if (i <= 3 || i + 1 >= nal.length || nal[i] !== 0x69) return null;

  try {
    const payload = stripEmulationBytes(nal.subarray(i + 1, nal.length - 1));
    return decodeSeiProtobuf(payload);
  } catch {
    return null;
  }
}

// ── MP4 Box Parser ──

export class DashcamMP4 {
  private buffer: ArrayBuffer;
  private view: DataView;
  private config: VideoConfig | null = null;

  constructor(buffer: ArrayBuffer) {
    this.buffer = buffer;
    this.view = new DataView(buffer);
  }

  private readAscii(start: number, len: number): string {
    let s = "";
    for (let i = 0; i < len; i++) {
      s += String.fromCharCode(this.view.getUint8(start + i));
    }
    return s;
  }

  private findBox(start: number, end: number, name: string): BoxLocation {
    for (let pos = start; pos + 8 <= end; ) {
      let size = this.view.getUint32(pos);
      const type = this.readAscii(pos + 4, 4);
      let headerSize = 8;

      if (size === 1) {
        // 64-bit extended size
        const high = this.view.getUint32(pos + 8);
        const low = this.view.getUint32(pos + 12);
        size = high * 0x100000000 + low;
        headerSize = 16;
      } else if (size === 0) {
        size = end - pos;
      }

      if (type === name) {
        return {
          start: pos + headerSize,
          end: pos + size,
          size: size - headerSize,
        };
      }
      pos += size;
    }
    throw new Error(`Box "${name}" not found`);
  }

  private findMdat(): { offset: number; size: number } {
    const mdat = this.findBox(0, this.view.byteLength, "mdat");
    return { offset: mdat.start, size: mdat.size };
  }

  private getConfig(): VideoConfig {
    if (this.config) return this.config;

    const moov = this.findBox(0, this.view.byteLength, "moov");
    const trak = this.findBox(moov.start, moov.end, "trak");
    const mdia = this.findBox(trak.start, trak.end, "mdia");

    // Timescale from mdhd
    const mdhd = this.findBox(mdia.start, mdia.end, "mdhd");
    const mdhdVersion = this.view.getUint8(mdhd.start);
    const timescale =
      mdhdVersion === 1
        ? this.view.getUint32(mdhd.start + 20)
        : this.view.getUint32(mdhd.start + 12);

    // Frame durations from stts
    const minf = this.findBox(mdia.start, mdia.end, "minf");
    const stbl = this.findBox(minf.start, minf.end, "stbl");
    const stts = this.findBox(stbl.start, stbl.end, "stts");
    const entryCount = this.view.getUint32(stts.start + 4);
    const durations: number[] = [];
    let readPos = stts.start + 8;
    for (let i = 0; i < entryCount; i++) {
      const count = this.view.getUint32(readPos);
      const delta = this.view.getUint32(readPos + 4);
      const ms = (delta / timescale) * 1000;
      for (let j = 0; j < count; j++) durations.push(ms);
      readPos += 8;
    }

    this.config = { timescale, durations };
    return this.config;
  }

  parseFrames(): SeiFrame[] {
    const config = this.getConfig();
    const mdat = this.findMdat();
    const frames: SeiFrame[] = [];
    let cursor = mdat.offset;
    const end = mdat.offset + mdat.size;
    let pendingSei: SeiMetadata | null = null;
    let timeMs = 0;

    while (cursor + 4 <= end) {
      const nalSize = this.view.getUint32(cursor);
      cursor += 4;
      if (nalSize < 1 || cursor + nalSize > this.buffer.byteLength) break;

      const nalType = this.view.getUint8(cursor) & 0x1f;
      const nalData = new Uint8Array(
        this.buffer.slice(cursor, cursor + nalSize),
      );

      if (nalType === 6) {
        pendingSei = decodeSeiNal(nalData);
      } else if (nalType === 5 || nalType === 1) {
        const frameIndex = frames.length;
        frames.push({
          time: timeMs / 1000,
          sei: pendingSei,
        });
        timeMs += config.durations[frameIndex] || 33;
        pendingSei = null;
      }

      cursor += nalSize;
    }

    return frames;
  }
}

// ── Frame lookup (binary search) ──

export function findSeiAtTime(
  frames: SeiFrame[],
  time: number,
): SeiMetadata | null {
  if (frames.length === 0) return null;

  let lo = 0;
  let hi = frames.length - 1;

  while (lo <= hi) {
    const mid = (lo + hi) >>> 1;
    if (frames[mid].time <= time) {
      lo = mid + 1;
    } else {
      hi = mid - 1;
    }
  }

  // hi is now the last frame with time <= target
  const idx = Math.max(0, hi);
  return frames[idx].sei;
}
