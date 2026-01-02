class ChitoSocket {
    constructor(url) {
        this.url = url;
        this.connected = false;
        this.socket = null;
        this.callbacks = {};
        this.pingInterval = null;

    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        this.socket = new WebSocket(`${protocol}//${this.url}/ws`);
        this.socket.onopen = () => {
            this.connected = true;
            this.createPingInterval();
            if (this.callbacks['connected']) {
                this.callbacks['connected']();
            }
        }

        this.socket.onerror = (event) => {
            if (this.callbacks['error']) {
                this.callbacks['error'](event);
            }
        }

        this.socket.onmessage = (event) => {
            const msg = JSON.parse(event.data);
            if (this.callbacks[msg.event]) {
                this.callbacks[msg.event](msg.data);
            }
        }

        this.socket.onclose = () => {
            this.connected = false;
            clearInterval(this.pingInterval);
            this.pingInterval = null;
            if (this.callbacks['disconnect']) {
                this.callbacks['disconnect']();
            }
            setTimeout(() => {
                this.connect();
            }, 2000);
        }
    }

    createPingInterval() {
        if (this.pingInterval !== null) {
            clearInterval(this.pingInterval);
        }
        this.pingInterval = setInterval(() => {
            this.send('ping', {});
        }, 10000);
    }
    
    send(event, data) {
        if (!this.connected) {
            return;
        }

        this.socket.send(JSON.stringify({
            event: event,
            data: data
        }));

        if (event !== 'ping') {
            this.createPingInterval();
        }
    }

    on(event, callback) {
        this.callbacks[event] = callback;
    }

    onConnect(callback) {
        this.on('connected', callback);
    }
    onDisconnect(callback) {
        this.on('disconnect', callback);
    }
    onError(callback) {
        this.on('error', callback);
    }

    close() {
        this.socket.close();
        clearInterval(this.pingInterval);
        this.pingInterval = null;
        this.connected = false;
        this.socket = null;
        this.callbacks = {};
    }
}