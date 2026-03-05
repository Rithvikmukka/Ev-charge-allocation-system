# EV Charging Dashboard

A web-based dashboard for managing and monitoring the distributed EV charging cluster.

## Features

- **Real-time cluster status**: View nodes, leader election, and health
- **Slot management**: Book, release, and monitor all charging slots
- **Demo automation**: One-click demos for all distributed features
- **Interactive UI**: Click slots to book/release, drag to resize
- **Live logs**: Real-time cluster activity feed
- **Multi-device support**: Works with localhost and multi-laptop deployments

## Quick Start

1. Start your cluster (see main README.md)
2. Open `web/index.html` in any browser
3. Enter the seed IP and port (e.g., `10.12.234.60:5001`)
4. Click **Connect**

## Usage

### Cluster Configuration
- Enter seed IP and port
- Click **Connect** to join the cluster
- View node status, leader, and membership

### Slot Management
- Click any slot to book it (enter vehicle ID)
- Click a booked slot to release it
- Use **Refresh** to update status
- Use **Release All** to clear all bookings

### Demo Controls
- **Normal Booking**: Books a single slot
- **Concurrent Booking**: Fires two requests simultaneously
- **Simulate Node Crash**: Instructions to stop a node
- **Simulate Leader Crash**: Triggers new election
- **Restart Node**: Instructions for crash recovery demo
- **Add New Node**: Instructions for scaling demo

### Manual Operations
- Enter slot and vehicle IDs
- Click **Book Slot** for manual booking

## API Endpoints

The dashboard uses these admin endpoints:

- `GET /healthz` - Health check
- `GET /admin/cluster` - Cluster membership
- `GET /admin/slots` - All slot states
- `POST /admin/election` - Trigger election
- `POST /admin/join?seed=<addr>` - Join cluster

## Files

- `index.html` - Main dashboard UI
- `api.js` - Cluster API client
- `style.css` - Responsive CSS styling

## Browser Support

Works in all modern browsers:
- Chrome 80+
- Firefox 75+
- Safari 13+
- Edge 80+

## Security Note

The dashboard connects to your cluster over HTTP. For production use, consider:
- HTTPS/TLS encryption
- API key authentication
- Network access controls
