"use client";

import { usePathname } from "next/navigation";
import VideosListPage from "./VideosList";
import VideoEventPage from "./VideoEvent";
import VideoSessionPage from "./VideoSession";

export default function VideosPage() {
  const pathname = usePathname();

  // pathname is like /videos, /videos/SavedClips, or /videos/SavedClips/2024-01-01_12-00-00
  const parts = pathname.replace(/^\/videos\/?/, "").split("/").filter(Boolean);

  // Archive folders are two segments ("archive/SavedClips"), so the event name
  // is one position further along than for the on-drive folders.
  const isArchive = parts[0] === "archive";
  const folder = isArchive ? `${parts[0]}/${parts[1]}` : parts[0];
  const second = isArchive ? parts[2] : parts[1];

  if (second) {
    // RecentClips holds sessions (no per-event subdirectory), archived or not.
    if (folder.endsWith("RecentClips")) {
      return <VideoSessionPage folder={folder} session={second} />;
    }
    return <VideoEventPage folder={folder} event={second} />;
  }

  return <VideosListPage />;
}
