"use client";

import { useEffect, useRef } from "react";
import type { SeiFrame, SeiMetadata } from "@/lib/sei-parser";
import { TILE_URLS, type MapTheme } from "@/lib/useMapTheme";
import "leaflet/dist/leaflet.css";

interface MapOverlayProps {
  seiFrames: SeiFrame[];
  currentSei: SeiMetadata | null;
  theme: MapTheme;
}

function hasGps(lat: number, lng: number): boolean {
  return !(lat === 0 && lng === 0);
}

export default function MapOverlay({ seiFrames, currentSei, theme }: MapOverlayProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<import("leaflet").Map | null>(null);
  const markerRef = useRef<import("leaflet").Marker | null>(null);
  const tileLayerRef = useRef<import("leaflet").TileLayer | null>(null);
  // Capture initial theme for the map init effect (which runs only once)
  const initialThemeRef = useRef<MapTheme>(theme);

  // Invalidate Leaflet map size when the container is resized by the user
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      mapRef.current?.invalidateSize();
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Initialize map once
  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;

    const L = require("leaflet") as typeof import("leaflet");

    // Fix Leaflet's default icon path (broken in bundlers)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete (L.Icon.Default.prototype as any)._getIconUrl;
    L.Icon.Default.mergeOptions({ iconRetinaUrl: "", iconUrl: "", shadowUrl: "" });

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

    tileLayerRef.current = L.tileLayer(TILE_URLS[initialThemeRef.current], { maxZoom: 19 }).addTo(map);

    if (points.length > 1) {
      L.polyline(points, { color: "#2e78ff", weight: 2.5, opacity: 0.8 }).addTo(map);
    }

    const iconHtml = `
      <div style="width:28px;height:28px;display:flex;align-items:center;justify-content:center;transform-origin:center;">
        <svg viewBox="0 0 24 24" width="28" height="28">
          <circle cx="12" cy="12" r="10" fill="rgba(46,120,255,0.25)" stroke="#2e78ff" stroke-width="1.5"/>
          <path d="M12 4 L15.5 16 L12 13.5 L8.5 16 Z" fill="#2e78ff"/>
        </svg>
      </div>
    `;

    const icon = L.divIcon({ html: iconHtml, className: "", iconSize: [28, 28], iconAnchor: [14, 14] });

    if (points.length > 0) {
      markerRef.current = L.marker(center, { icon }).addTo(map);
    }

    mapRef.current = map;

    return () => {
      map.remove();
      mapRef.current = null;
      markerRef.current = null;
      tileLayerRef.current = null;
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Swap tile layer when theme changes (skip on first render; init already set it)
  const isFirstThemeRender = useRef(true);
  useEffect(() => {
    if (isFirstThemeRender.current) { isFirstThemeRender.current = false; return; }
    const map = mapRef.current;
    if (!map) return;
    const L = require("leaflet") as typeof import("leaflet");
    if (tileLayerRef.current) map.removeLayer(tileLayerRef.current);
    tileLayerRef.current = L.tileLayer(TILE_URLS[theme], { maxZoom: 19 }).addTo(map);
  }, [theme]);

  // Update marker position and heading as currentSei changes
  useEffect(() => {
    const map = mapRef.current;
    const marker = markerRef.current;
    if (!map || !currentSei) return;
    if (!hasGps(currentSei.latitudeDeg, currentSei.longitudeDeg)) return;

    const latlng: [number, number] = [currentSei.latitudeDeg, currentSei.longitudeDeg];

    if (marker) {
      marker.setLatLng(latlng);
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
        width: "220px",
        height: "165px",
        minWidth: "140px",
        minHeight: "100px",
        maxWidth: "480px",
        maxHeight: "360px",
        resize: "both",
        overflow: "hidden",
        borderRadius: "10px",
        border: "1px solid rgba(255,255,255,0.15)",
        boxShadow: "0 4px 16px rgba(0,0,0,0.5)",
        background: "#1a1a2e",
        cursor: "default",
      }}
    />
  );
}
