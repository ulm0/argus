import type { Metadata, Viewport } from "next";
import { Inter } from "next/font/google";
import { ThemeProvider } from "@/components/ThemeProvider";
import AuthGate from "@/components/AuthGate";
import Sidebar from "@/components/Sidebar";
import SystemPanel from "@/components/SystemPanel";
import SentryAlert from "@/components/SentryAlert";
import { ToastProvider } from "@/components/Toast";
import "./globals.css";

const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin"],
  display: "swap",
});

export const metadata: Metadata = {
  title: "Argus",
  description: "Dashcam & Sentry Mode manager for Tesla vehicles",
  icons: { icon: "/favicon.ico" },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  // Required for env(safe-area-inset-*) to resolve, so fixed-position toasts and
  // alerts clear the home indicator on notched phones.
  viewportFit: "cover",
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#fafafa" },
    { media: "(prefers-color-scheme: dark)", color: "#171717" },
  ],
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className={`${inter.variable} h-full antialiased`} suppressHydrationWarning>
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `(function(){try{var t=localStorage.getItem("argus-theme");if(t==="dark"||(t!=="light"&&matchMedia("(prefers-color-scheme:dark)").matches))document.documentElement.classList.add("dark")}catch(e){}})()`,
          }}
        />
      </head>
      <body className="flex h-screen overflow-hidden bg-[var(--color-bg-secondary)] text-[var(--color-text-primary)] font-[family-name:var(--font-inter)]">
        <ThemeProvider>
          <AuthGate>
            <ToastProvider>
              <Sidebar />
              {/* pt-16 clears Sidebar's fixed hamburger, which otherwise sits on
                  every page's <h1> below lg. */}
              <main className="flex-1 overflow-y-auto pt-16 lg:pt-0">{children}</main>
              <SystemPanel />
              <SentryAlert />
            </ToastProvider>
          </AuthGate>
        </ThemeProvider>
      </body>
    </html>
  );
}
