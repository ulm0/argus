"use client";

const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"] as const;

function format(bytes: number): string {
  if (bytes === 0) return "0 B";
  const exp = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), UNITS.length - 1);
  const value = bytes / 1024 ** exp;
  return `${value < 10 ? value.toFixed(2) : value < 100 ? value.toFixed(1) : Math.round(value)} ${UNITS[exp]}`;
}

interface FileSizeProps {
  bytes: number;
}

export default function FileSize({ bytes }: FileSizeProps) {
  return <span>{format(bytes)}</span>;
}

export { format as formatBytes };
