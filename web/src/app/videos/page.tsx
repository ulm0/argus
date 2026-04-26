"use client";

import { usePathname } from "next/navigation";
import VideosListPage from "./VideosList";
import VideoEventPage from "./VideoEvent";
import VideoSessionPage from "./VideoSession";

export default function VideosPage() {
  const pathname = usePathname();

  // pathname is like /videos, /videos/SavedClips, or /videos/SavedClips/2024-01-01_12-00-00
  const parts = pathname.replace(/^\/videos\/?/, "").split("/").filter(Boolean);

  if (parts.length >= 2) {
    const folder = parts[0];
    const second = parts[1];
    // RecentClips sessions are identified by timestamp format (no subdirectory events)
    if (folder === "RecentClips" || folder.endsWith("/RecentClips")) {
      return <VideoSessionPage folder={folder} session={second} />;
    }
    return <VideoEventPage folder={folder} event={second} />;
  }

  return <VideosListPage />;
}
