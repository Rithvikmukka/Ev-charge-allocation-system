class ClusterAPI {
    constructor(seedIp, seedPort) {
        this.seedIp = seedIp;
        this.seedPort = seedPort;
        this.baseUrl = `http://${seedIp}:${seedPort}`;
    }

    async request(method, endpoint, body = null) {
        const url = `${this.baseUrl}${endpoint}`;
        const options = {
            method: method,
            headers: {
                'Content-Type': 'application/json',
            }
        };
        
        if (body) {
            options.body = JSON.stringify(body);
        }

        try {
            const response = await fetch(url, options);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            return await response.json();
        } catch (error) {
            console.error(`API Error: ${error.message}`);
            throw error;
        }
    }

    // Cluster management
    async getMembership() {
        return this.request('GET', '/membership');
    }

    async getHealth() {
        try {
            await this.request('GET', '/healthz');
            return true;
        } catch {
            return false;
        }
    }

    // Slot operations
    async getSlot(slotKey) {
        return this.request('GET', `/slot?slot=${slotKey}`);
    }

    async getAllSlots() {
        const slots = [
            'StationA-Slot1', 'StationA-Slot2', 'StationA-Slot3',
            'StationB-Slot1', 'StationB-Slot2', 'StationB-Slot3',
            'StationC-Slot1', 'StationC-Slot2', 'StationC-Slot3'
        ];
        
        const promises = slots.map(slot => 
            this.getSlot(slot).catch(err => ({ slotKey: slot, error: err.message }))
        );
        
        return Promise.all(promises);
    }

    async reserveSlot(slotKey, vehicleId) {
        return this.request('POST', '/reserve', { slot: slotKey, vehicle: vehicleId });
    }

    async releaseSlot(slotKey, vehicleId) {
        return this.request('POST', '/release', { slot: slotKey, vehicle: vehicleId });
    }

    // Demo operations
    async triggerElection() {
        return this.request('POST', '/election/start');
    }

    async joinCluster(seedAddr) {
        return this.request('POST', `/join?seed=${seedAddr}`);
    }
}

// Global state
let api = null;
let refreshInterval = null;

// UI Functions
function log(message, type = 'info') {
    const logs = document.getElementById('logs');
    const timestamp = new Date().toLocaleTimeString();
    const logEntry = document.createElement('div');
    logEntry.className = 'log-entry';
    logEntry.innerHTML = `<span class="log-time">[${timestamp}]</span> ${message}`;
    logs.appendChild(logEntry);
    logs.scrollTop = logs.scrollHeight;
}

function updateClusterStatus(status, message = '') {
    const statusEl = document.getElementById('clusterStatus');
    statusEl.className = `status-item status-${status}`;
    statusEl.textContent = message;
}

function renderNodes(membership) {
    const nodeList = document.getElementById('nodeList');
    nodeList.innerHTML = '';
    
    if (!membership || !membership.members) {
        nodeList.innerHTML = '<div style="color: #666; text-align: center; padding: 20px;">No nodes available</div>';
        return;
    }

    const nodes = membership.members.map(addr => {
        const parts = addr.split(':');
        return {
            addr: addr,
            ip: parts[0],
            port: parts[1] || '5001',
            id: membership.peerIds[addr] || '?'
        };
    });

    nodes.sort((a, b) => (b.id || 0) - (a.id || 0));

    nodes.forEach(node => {
        const nodeEl = document.createElement('div');
        nodeEl.className = 'node';
        
        const isLeader = node.id === Math.max(...nodes.map(n => n.id || 0));
        if (isLeader) nodeEl.classList.add('leader');
        
        nodeEl.innerHTML = `
            <div>
                <div class="node-id">Node ${node.id}</div>
                <div style="font-size: 12px; color: #666;">${node.ip}:${node.port}</div>
            </div>
            <div>
                <span class="node-status ${isLeader ? 'status-healthy' : 'status-warning'}">
                    ${isLeader ? 'LEADER' : 'FOLLOWER'}
                </span>
            </div>
        `;
        
        nodeList.appendChild(nodeEl);
    });

    document.getElementById('nodeCount').textContent = `${nodes.length} nodes`;
    
    const leader = nodes.find(n => n.id === Math.max(...nodes.map(n => n.id || 0)));
    document.getElementById('leaderInfo').textContent = leader ? `Leader: Node ${leader.id}` : 'No leader';
}

function renderSlots(slots) {
    const slotsGrid = document.getElementById('slotsGrid');
    slotsGrid.innerHTML = '';

    slots.forEach(slot => {
        const slotEl = document.createElement('div');
        slotEl.className = `slot ${slot.error ? 'status-error' : (slot.slot?.Status === 'BOOKED' ? 'booked' : 'free')}`;
        slotEl.onclick = () => handleSlotClick(slot);
        
        if (slot.error) {
            slotEl.innerHTML = `
                <div class="slot-name">${slot.slotKey}</div>
                <div style="color: #f44336; font-size: 12px;">Error</div>
            `;
        } else {
            slotEl.innerHTML = `
                <div class="slot-name">${slot.slotKey}</div>
                <div class="slot-vehicle">${slot.slot?.VehicleID || 'FREE'}</div>
                <div style="font-size: 10px; color: #666;">v${slot.slot?.Version || 0}</div>
            `;
        }
        
        slotsGrid.appendChild(slotEl);
    });
}

async function handleSlotClick(slot) {
    if (slot.error || !slot.slot) return;
    
    if (slot.slot.Status === 'BOOKED') {
        if (confirm(`Release ${slot.slotKey} from ${slot.slot.VehicleID}?`)) {
            try {
                await api.releaseSlot(slot.slotKey, slot.slot.VehicleID);
                log(`Released ${slot.slotKey} from ${slot.slot.VehicleId}`);
                await refreshSlots();
            } catch (error) {
                log(`Failed to release ${slot.slotKey}: ${error.message}`, 'error');
            }
        }
    } else {
        const vehicleId = prompt(`Book ${slot.slotKey} for vehicle:`, 'EV' + Math.floor(Math.random() * 1000));
        if (vehicleId) {
            try {
                await api.reserveSlot(slot.slotKey, vehicleId);
                log(`Booked ${slot.slotKey} for ${vehicleId}`);
                await refreshSlots();
            } catch (error) {
                log(`Failed to book ${slot.slotKey}: ${error.message}`, 'error');
            }
        }
    }
}

async function connect() {
    const seedIp = document.getElementById('seedIp').value;
    const seedPort = document.getElementById('seedPort').value;
    
    if (!seedIp || !seedPort) {
        alert('Please enter seed IP and port');
        return;
    }

    api = new ClusterAPI(seedIp, seedPort);
    
    try {
        const isHealthy = await api.getHealth();
        if (isHealthy) {
            updateClusterStatus('healthy', 'Connected');
            log(`Connected to cluster at ${seedIp}:${seedPort}`);
            await refreshCluster();
            startAutoRefresh();
        } else {
            updateClusterStatus('error', 'Connection failed');
            log(`Failed to connect to ${seedIp}:${seedPort}`, 'error');
        }
    } catch (error) {
        updateClusterStatus('error', 'Connection failed');
        log(`Failed to connect: ${error.message}`, 'error');
    }
}

async function refreshCluster() {
    if (!api) return;
    
    try {
        const membership = await api.getMembership();
        renderNodes(membership);
        await refreshSlots();
    } catch (error) {
        log(`Failed to refresh cluster: ${error.message}`, 'error');
        updateClusterStatus('error', 'Connection lost');
    }
}

async function refreshSlots() {
    if (!api) return;
    
    try {
        const slots = await api.getAllSlots();
        renderSlots(slots);
    } catch (error) {
        log(`Failed to refresh slots: ${error.message}`, 'error');
    }
}

async function bookSlot() {
    if (!api) {
        alert('Please connect to cluster first');
        return;
    }
    
    const slotKey = document.getElementById('slotInput').value;
    const vehicleId = document.getElementById('vehicleInput').value;
    
    if (!slotKey || !vehicleId) {
        alert('Please enter slot and vehicle ID');
        return;
    }

    try {
        await api.reserveSlot(slotKey, vehicleId);
        log(`Booked ${slotKey} for ${vehicleId}`);
        await refreshSlots();
    } catch (error) {
        log(`Failed to book ${slotKey}: ${error.message}`, 'error');
    }
}

async function releaseAllSlots() {
    if (!api) return;
    
    if (!confirm('Release all booked slots?')) return;
    
    try {
        const slots = await api.getAllSlots();
        const releasePromises = slots
            .filter(slot => !slot.error && slot.slot?.Status === 'BOOKED')
            .map(slot => api.releaseSlot(slot.slotKey, slot.slot.VehicleId));
        
        await Promise.all(releasePromises);
        log('Released all slots');
        await refreshSlots();
    } catch (error) {
        log(`Failed to release slots: ${error.message}`, 'error');
    }
}

// Demo functions
async function demoNormalBooking() {
    if (!api) return;
    
    log('Starting normal booking demo...');
    
    try {
        await api.reserveSlot('StationA-Slot1', 'EV-DEMO-001');
        log('✓ Booked StationA-Slot1 for EV-DEMO-001');
        await refreshSlots();
    } catch (error) {
        log(`✗ Normal booking failed: ${error.message}`, 'error');
    }
}

async function demoConcurrentBooking() {
    if (!api) return;
    
    log('Starting concurrent booking demo...');
    
    try {
        // Fire two concurrent requests for the same slot
        const slot = 'StationB-Slot1';
        const promises = [
            api.reserveSlot(slot, 'EV-DEMO-201'),
            api.reserveSlot(slot, 'EV-DEMO-202')
        ];
        
        const results = await Promise.allSettled(promises);
        const success = results.filter(r => r.status === 'fulfilled').length;
        const failed = results.filter(r => r.status === 'rejected').length;
        
        log(`✓ Concurrent booking: ${success} succeeded, ${failed} failed (as expected)`);
        await refreshSlots();
    } catch (error) {
        log(`✗ Concurrent booking demo failed: ${error.message}`, 'error');
    }
}

async function demoNodeCrash() {
    log('⚠️ Node crash demo: Stop a node manually (Ctrl+C) to see heartbeat failure detection');
    log('   The cluster will mark the node as FAILED after ~5 seconds');
}

async function demoLeaderCrash() {
    if (!api) return;
    
    log('⚠️ Leader crash demo: Triggering new election...');
    
    try {
        await api.triggerElection();
        log('✓ Election triggered - watch logs for leader changes');
    } catch (error) {
        log(`✗ Election failed: ${error.message}`, 'error');
    }
}

async function demoNodeRestart() {
    log('⚠️ Node restart demo: Start a stopped node to see crash recovery');
    log('   The node will sync latest state from peers');
}

async function demoAddNode() {
    log('⚠️ Add node demo: Start a new node with --auto-join to see scaling');
    log('   Command: go run ./cmd --peers=<seedIP> --auto-join');
}

function clearLogs() {
    document.getElementById('logs').innerHTML = '';
    log('Logs cleared');
}

function refreshLogs() {
    log('Manual refresh triggered');
    refreshCluster();
}

function startAutoRefresh() {
    if (refreshInterval) clearInterval(refreshInterval);
    refreshInterval = setInterval(refreshCluster, 5000);
}

function stopAutoRefresh() {
    if (refreshInterval) {
        clearInterval(refreshInterval);
        refreshInterval = null;
    }
}

// Initialize on load
document.addEventListener('DOMContentLoaded', () => {
    log('Dashboard loaded - click Connect to join cluster');
});

// Cleanup on unload
window.addEventListener('beforeunload', () => {
    stopAutoRefresh();
});
