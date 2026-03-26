# Argus: Edge-Optimized Dashcam Manager

## Project Overview
Argus is a lightweight, edge-computing solution designed to store, manage, and process Dashcam and Sentry Mode data from Tesla vehicles. The core objective is to deliver a high-performance, resource-efficient application that runs seamlessly on low-power edge devices.

## Architecture & Tech Stack
- **Backend:** Pure Golang.
- **Frontend:** NextJS.
- **Delivery:** The entire application (NextJS frontend included) MUST be compiled and embedded into a single, small-footprint Golang binary.
- **Target Hardware:** Must be fully operational on a Raspberry Pi Zero 2 W (or equivalent edge devices) with strictly limited CPU and RAM availability.

## UI & Frontend Specifications
- **Theming:** The UI design MUST include fully functional Light and Dark modes.
- **Efficiency:** The frontend must be highly responsive, lightweight, and optimized for both mobile and desktop viewing without taxing the host device.

## Performance Constraints & Guidelines
- **Resource Minimalism:** The application must consume the absolute minimum RAM and CPU possible.
- **Dependency Management:** Zero external dependencies outside the Golang and NextJS standard ecosystems unless strictly required by the OS (e.g., `systemd`, unit files for daemonization).

## Core Features & Parity
- **Feature Parity:** The system must implement all core functionalities of the legacy repository (`https://github.com/mphacker/TeslaUSB`) entirely in Go.
- **Single Source of Truth:** All application configurations, environmental variables, and states must be governed by a centralized `config.yaml` file.
- **Real-Time Alerting & Connectivity:** The system must detect Sentry Mode events and trigger instant Telegram notifications, attaching high-definition (HD) video clips of the event. **Crucially, this alerting mechanism MUST execute if and only if an active internet connection is detected.** The system must handle offline states gracefully (e.g., queuing alerts or discarding them safely based on configuration) without crashing or blocking other processes.

## Multi-Agent Workflow & Code Quality Strict Rules
- **Functional Delivery:** All delivered code MUST be fully functional out of the box. Partial or broken implementations are strictly forbidden.
- **Mandatory Testing:** Every single piece of code generated MUST be rigorously tested.
- **Concurrency Control:** Multi-agent collaboration is allowed and encouraged, PROVIDED that strict file-locking mechanisms are respected. Agents MUST NOT edit the same files simultaneously to prevent overlaps and race conditions.
- **Analytical Refactoring:** Every piece of code must be critically analyzed upon creation. If an initial solution is flawed or non-functional, the agent MUST independently reimplement or refactor the code entirely to guarantee a working final product.
