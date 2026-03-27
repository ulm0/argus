"use client";

import { usePathname } from "next/navigation";
import VideosListPage from "./VideosList";
import VideoEventPage from "./VideoEvent";

export default function VideosPage() {
  const pathname = usePathname();

  // pathname is like /videos, /videos/SavedClips, or /videos/SavedClips/2024-01-01_12-00-00
  const parts = pathname.replace(/^\/videos\/?/, "").split("/").filter(Boolean);

  if (parts.length >= 2) {
    return <VideoEventPage folder={parts[0]} event={parts[1]} />;
  }

  return <VideosListPage />;
}
