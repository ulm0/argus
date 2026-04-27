"use client";

import { useEffect, useRef } from "react";
import type { SeiFrame, SeiMetadata } from "@/lib/sei-parser";
import "leaflet/dist/leaflet.css";

interface MapOverlayProps {
  seiFrames: SeiFrame[];
  currentSei: SeiMetadata | null;
}

function hasGps(lat: number, lng: number): boolean {
  return !(lat === 0 && lng === 0);
}

export default function MapOverlay({ seiFrames, currentSei }: MapOverlayProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<import("leaflet").Map | null>(null);
  const markerRef = useRef<import("leaflet").Marker | null>(null);

  // Initialize map once
  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;

    // Leaflet is only safe to use client-side (this component is dynamic-imported with ssr:false)
    const L = require("leaflet") as typeof import("leaflet");

    // Fix Leaflet's default icon path (broken in bundlers)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete (L.Icon.Default.prototype as any)._getIconUrl;
    L.Icon.Default.mergeOptions({
      iconRetinaUrl: "",
      iconUrl: "",
      shadowUrl: "",
    });

    // Collect route points
    const points: [number, number][] = [];
    for (const frame of seiFrames) {
      if (frame.sei && hasGps(frame.sei.latitudeDeg, frame.sei.longitudeDeg)) {
        points.push([frame.sei.latitudeDeg, frame.sei.longitudeDeg]);
      }
    }

    const center: [number, number] = points.length > 0 ? points[0] : [0, 0];
    const zoom = points.length > 0 ? 15 : 2;

    const map = L.map(containerRef.current, {
      center,
      zoom,
      zoomControl: false,
      attributionControl: false,
      dragging: false,
      scrollWheelZoom: false,
      doubleClickZoom: false,
      keyboard: false,
      touchZoom: false,
      boxZoom: false,
    });

    L.tileLayer(
      "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png",
      { maxZoom: 19 },
    ).addTo(map);

    // Draw route polyline
    if (points.length > 1) {
      L.polyline(points, {
        color: "#2e78ff",
        weight: 2.5,
        opacity: 0.8,
      }).addTo(map);
    }

    // Heading marker icon (SVG arrow)
    const iconHtml = `
      <div style="
        width:28px; height:28px;
        display:flex; align-items:center; justify-content:center;
        transform-origin:center;
      ">
        <svg viewBox="0 0 24 24" width="28" height="28">
          <circle cx="12" cy="12" r="10" fill="rgba(46,120,255,0.25)" stroke="#2e78ff" stroke-width="1.5"/>
          <path d="M12 4 L15.5 16 L12 13.5 L8.5 16 Z" fill="#2e78ff"/>
        </svg>
      </div>
    `;

    const icon = L.divIcon({
      html: iconHtml,
      className: "",
      iconSize: [28, 28],
      iconAnchor: [14, 14],
    });

    if (points.length > 0) {
      const marker = L.marker(center, { icon }).addTo(map);
      markerRef.current = marker;
    }

    mapRef.current = map;

    return () => {
      map.remove();
      mapRef.current = null;
      markerRef.current = null;
    };
  // Only run once on mount; seiFrames is stable (loaded once per file)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Update marker position and heading as currentSei changes
  useEffect(() => {
    const map = mapRef.current;
    const marker = markerRef.current;
    if (!map || !currentSei) return;
    if (!hasGps(currentSei.latitudeDeg, currentSei.longitudeDeg)) return;

    const latlng: [number, number] = [currentSei.latitudeDeg, currentSei.longitudeDeg];

    if (marker) {
      marker.setLatLng(latlng);
      // Rotate the inner SVG arrow to reflect heading
      const el = marker.getElement();
      if (el) {
        const inner = el.querySelector("div") as HTMLElement | null;
        if (inner) inner.style.transform = `rotate(${currentSei.headingDeg}deg)`;
      }
    }

    map.panTo(latlng, { animate: true, duration: 0.3 });
  }, [currentSei]);

  return (
    <div
      ref={containerRef}
      style={{
        width: "160px",
        height: "120px",
        borderRadius: "10px",
        overflow: "hidden",
        border: "1px solid rgba(255,255,255,0.15)",
        boxShadow: "0 4px 16px rgba(0,0,0,0.5)",
        background: "#1a1a2e",
      }}
    />
  );
}
