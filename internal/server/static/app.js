/**
 * Warp Upload - Web UI JavaScript
 * Implements file upload functionality with TUI-consistent styling
 */

// ============================================================================
// State Management
// ============================================================================

/** @type {Array<FileQueueItem>} */
const fileQueue = [];

/** @type {Map<string, UploadState>} */
const uploadStates = new Map();

/** @type {ConnectionState} */
const connectionState = {
  websocket: "disconnected",
  server: "offline",
  lastPing: 0,
};

/** @type {WebSocket|null} */
let ws = null;

/** @type {number} */
let reconnectAttempts = 0;

/** @type {number|null} */
let reconnectTimeout = null;

/** @type {number|null} */
let healthPollInterval = null;

/** @type {{chunkSize: number, maxConcurrent: number}} */
let manifest = {
  chunkSize: 2 * 1024 * 1024, // 2MB default
  maxConcurrent: 3,
};

// ============================================================================
// DOM Elements
// ============================================================================

const uploadZone = document.getElementById("upload-zone");
const fileInput = document.getElementById("file-input");
const fileList = document.getElementById("file-list");
const emptyState = document.getElementById("empty-state");
const uploadBtn = document.getElementById("upload-btn");
const connectionStatus = document.getElementById("connection-status");
const connectionText = document.getElementById("connection-text");
const statusText = document.getElementById("status-text");
const statusIndicator = document.getElementById("status-indicator");
const userIp = document.getElementById("user-ip");

// Templates
const fileItemTemplate = document.getElementById("file-item-template");
const fileCompleteTemplate = document.getElementById("file-complete-template");
const fileErrorTemplate = document.getElementById("file-error-template");

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Format bytes to human-readable size
 * @param {number} bytes
 * @returns {string}
 */
function formatSize(bytes) {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const size = bytes / Math.pow(1024, i);
  return `${size.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

/**
 * Generate unique ID for file queue items
 * @returns {string}
 */
function generateId() {
  return `file-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
}

/**
 * Calculate progress percentage
 * @param {number} completedBytes
 * @param {number} totalBytes
 * @returns {number}
 */
function calculateProgress(completedBytes, totalBytes) {
  if (totalBytes <= 0) return 0;
  return Math.round((completedBytes / totalBytes) * 100);
}

/**
 * Format speed in Mbps
 * @param {number} bytesPerSecond
 * @returns {string}
 */
function formatSpeed(bytesPerSecond) {
  const mbps = (bytesPerSecond * 8) / 1_000_000;
  return `${mbps.toFixed(1)} Mbps`;
}

// ============================================================================
// File Queue Management
// ============================================================================

/**
 * Add files to the queue
 * @param {FileList|File[]} files
 */
function addFiles(files) {
  for (const file of files) {
    // Skip empty files
    if (file.size === 0) {
      showNotification(`Skipped empty file: ${file.name}`, "warning");
      continue;
    }

    const item = {
      id: generateId(),
      file: file,
      name: file.name,
      size: file.size,
      status: "queued",
      progress: 0,
      speed: 0,
      sessionId: null,
      error: null,
    };

    fileQueue.push(item);
  }

  renderFileQueue();
  updateUploadButton();
}

/**
 * Remove a file from the queue
 * @param {string} fileId
 */
function removeFile(fileId) {
  const index = fileQueue.findIndex((f) => f.id === fileId);
  if (index === -1) return;

  const item = fileQueue[index];

  // Cancel upload if in progress
  if (item.status === "uploading" || item.status === "paused") {
    cancelUpload(fileId);
  }

  // Remove from queue
  fileQueue.splice(index, 1);

  // Clean up upload state
  uploadStates.delete(fileId);

  renderFileQueue();
  updateUploadButton();
}

/**
 * Clear all files from the queue
 */
function clearQueue() {
  // Cancel all active uploads
  for (const item of fileQueue) {
    if (item.status === "uploading" || item.status === "paused") {
      cancelUpload(item.id);
    }
  }

  fileQueue.length = 0;
  uploadStates.clear();

  renderFileQueue();
  updateUploadButton();
}

/**
 * Render the file queue to the DOM
 */
function renderFileQueue() {
  // Clear existing items
  fileList.innerHTML = "";

  // Show/hide empty state
  if (fileQueue.length === 0) {
    emptyState.classList.remove("hidden");
    fileList.classList.add("hidden");
    return;
  }

  emptyState.classList.add("hidden");
  fileList.classList.remove("hidden");

  // Render each file
  for (const item of fileQueue) {
    const element = createFileElement(item);
    fileList.appendChild(element);
  }
}

/**
 * Create DOM element for a file queue item
 * @param {FileQueueItem} item
 * @returns {HTMLElement}
 */
function createFileElement(item) {
  let template;

  if (item.status === "complete") {
    template = fileCompleteTemplate;
  } else if (item.status === "error") {
    template = fileErrorTemplate;
  } else {
    template = fileItemTemplate;
  }

  const clone = template.content.cloneNode(true);
  const element = clone.querySelector(".file-item");

  // Set data attributes
  element.dataset.fileId = item.id;
  element.dataset.status = item.status;

  // Set file info
  element.querySelector(".file-name").textContent = item.name;
  element.querySelector(".file-name").title = item.name;
  element.querySelector(".file-size").textContent = formatSize(item.size);

  // Set progress
  const progressBar = element.querySelector(".progress-bar");
  const progressContainer = element.querySelector(".progress-container");
  if (progressBar) {
    progressBar.style.width = `${item.progress}%`;
  }
  if (progressContainer) {
    progressContainer.setAttribute("aria-valuenow", item.progress);
  }

  // Set progress text
  const progressText = element.querySelector(".file-progress-text");
  if (progressText && item.status !== "error") {
    progressText.textContent = `${item.progress}%`;
  }

  // Set speed
  const speedElement = element.querySelector(".file-speed");
  if (speedElement && item.status === "uploading") {
    speedElement.textContent = formatSpeed(item.speed);
  }

  // Set error message
  if (item.status === "error") {
    const errorElement = element.querySelector(".file-error");
    if (errorElement) {
      errorElement.textContent = item.error || "Upload failed";
    }
  }

  // Show/hide pause/resume buttons based on status
  const pauseBtn = element.querySelector(".btn-pause");
  const resumeBtn = element.querySelector(".btn-resume");
  if (pauseBtn && resumeBtn) {
    if (item.status === "uploading") {
      pauseBtn.classList.remove("hidden");
      resumeBtn.classList.add("hidden");
    } else if (item.status === "paused") {
      pauseBtn.classList.add("hidden");
      resumeBtn.classList.remove("hidden");
    } else {
      pauseBtn.classList.add("hidden");
      resumeBtn.classList.add("hidden");
    }
  }

  // Attach event listeners
  attachFileEventListeners(element, item.id);

  return element;
}

/**
 * Attach event listeners to file element buttons
 * @param {HTMLElement} element
 * @param {string} fileId
 */
function attachFileEventListeners(element, fileId) {
  const pauseBtn = element.querySelector(".btn-pause");
  const resumeBtn = element.querySelector(".btn-resume");
  const removeBtn = element.querySelector(".btn-remove");
  const retryBtn = element.querySelector(".btn-retry");

  if (pauseBtn) {
    pauseBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      pauseUpload(fileId);
    });
  }

  if (resumeBtn) {
    resumeBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      resumeUpload(fileId);
    });
  }

  if (removeBtn) {
    removeBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      removeFile(fileId);
    });
  }

  if (retryBtn) {
    retryBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      retryUpload(fileId);
    });
  }
}

/**
 * Update a specific file's display
 * @param {string} fileId
 */
function updateFileDisplay(fileId) {
  const item = fileQueue.find((f) => f.id === fileId);
  if (!item) return;

  const element = fileList.querySelector(`[data-file-id="${fileId}"]`);
  if (!element) {
    renderFileQueue();
    return;
  }

  // Update progress bar
  const progressBar = element.querySelector(".progress-bar");
  const progressContainer = element.querySelector(".progress-container");
  if (progressBar) {
    progressBar.style.width = `${item.progress}%`;
  }
  if (progressContainer) {
    progressContainer.setAttribute("aria-valuenow", item.progress);
  }

  // Update progress text
  const progressText = element.querySelector(".file-progress-text");
  if (progressText) {
    progressText.textContent = `${item.progress}%`;
  }

  // Update speed
  const speedElement = element.querySelector(".file-speed");
  if (speedElement) {
    speedElement.textContent =
      item.status === "uploading" ? formatSpeed(item.speed) : "";
  }

  // Update status attribute
  element.dataset.status = item.status;

  // Update pause/resume buttons
  const pauseBtn = element.querySelector(".btn-pause");
  const resumeBtn = element.querySelector(".btn-resume");
  if (pauseBtn && resumeBtn) {
    if (item.status === "uploading") {
      pauseBtn.classList.remove("hidden");
      resumeBtn.classList.add("hidden");
    } else if (item.status === "paused") {
      pauseBtn.classList.add("hidden");
      resumeBtn.classList.remove("hidden");
    }
  }

  // If status changed to complete or error, re-render
  if (item.status === "complete" || item.status === "error") {
    renderFileQueue();
  }
}

/**
 * Update upload button state
 */
function updateUploadButton() {
  const hasQueuedFiles = fileQueue.some((f) => f.status === "queued");
  const hasActiveUploads = fileQueue.some(
    (f) => f.status === "uploading" || f.status === "paused",
  );

  uploadBtn.disabled = !hasQueuedFiles && !hasActiveUploads;

  if (hasActiveUploads) {
    uploadBtn.textContent = "[ UPLOADING... ]";
  } else if (hasQueuedFiles) {
    uploadBtn.textContent = "[ START UPLOAD ]";
  } else {
    uploadBtn.textContent = "[ START UPLOAD ]";
  }
}

// ============================================================================
// Upload Manager
// ============================================================================

/**
 * Start uploading all queued files
 */
function startUpload() {
  const queuedFiles = fileQueue.filter((f) => f.status === "queued");

  for (const item of queuedFiles) {
    startFileUpload(item.id);
  }

  updateUploadButton();
  updateStatus("Uploading...");
}

/**
 * Start uploading a specific file
 * @param {string} fileId
 */
function startFileUpload(fileId) {
  const item = fileQueue.find((f) => f.id === fileId);
  if (!item || item.status !== "queued") return;

  // Initialize upload state
  const state = {
    chunkSize: manifest.chunkSize,
    maxConcurrent: manifest.maxConcurrent,
    pending: [],
    inFlight: new Set(),
    completedBytes: 0,
    startTime: Date.now(),
    xhrs: new Map(),
    isPaused: false,
  };

  // Calculate chunks
  const totalChunks = Math.ceil(item.size / state.chunkSize);
  for (let i = 0; i < totalChunks; i++) {
    state.pending.push(i);
  }

  // Generate session ID
  item.sessionId = generateId();
  item.status = "uploading";

  uploadStates.set(fileId, state);

  // Start uploading chunks
  scheduleChunks(fileId);
  updateFileDisplay(fileId);
}

/**
 * Pause upload for a specific file
 * @param {string} fileId
 */
function pauseUpload(fileId) {
  const item = fileQueue.find((f) => f.id === fileId);
  const state = uploadStates.get(fileId);

  if (!item || !state) return;
  if (item.status !== "uploading") return;

  state.isPaused = true;
  item.status = "paused";

  // Abort in-flight requests
  for (const [chunkId, xhr] of state.xhrs) {
    xhr.abort();
    // Re-add to pending
    if (!state.pending.includes(chunkId)) {
      state.pending.unshift(chunkId);
    }
  }
  state.xhrs.clear();
  state.inFlight.clear();

  updateFileDisplay(fileId);
  updateUploadButton();
  updateStatus("Paused");
}

/**
 * Resume upload for a specific file
 * @param {string} fileId
 */
function resumeUpload(fileId) {
  const item = fileQueue.find((f) => f.id === fileId);
  const state = uploadStates.get(fileId);

  if (!item || !state) return;
  if (item.status !== "paused") return;

  state.isPaused = false;
  item.status = "uploading";

  // Resume uploading
  scheduleChunks(fileId);
  updateFileDisplay(fileId);
  updateUploadButton();
  updateStatus("Uploading...");
}

/**
 * Cancel upload for a specific file
 * @param {string} fileId
 */
function cancelUpload(fileId) {
  const item = fileQueue.find((f) => f.id === fileId);
  const state = uploadStates.get(fileId);

  if (!item) return;

  if (state) {
    state.isPaused = true;

    // Abort all in-flight requests
    for (const xhr of state.xhrs.values()) {
      xhr.abort();
    }
    state.xhrs.clear();
    state.inFlight.clear();
  }

  item.status = "queued";
  item.progress = 0;
  item.speed = 0;

  uploadStates.delete(fileId);
  updateFileDisplay(fileId);
  updateUploadButton();
}

/**
 * Retry a failed upload
 * @param {string} fileId
 */
function retryUpload(fileId) {
  const item = fileQueue.find((f) => f.id === fileId);
  if (!item || item.status !== "error") return;

  item.status = "queued";
  item.progress = 0;
  item.speed = 0;
  item.error = null;

  uploadStates.delete(fileId);
  startFileUpload(fileId);
}

/**
 * Schedule chunk uploads respecting concurrency limit
 * @param {string} fileId
 */
function scheduleChunks(fileId) {
  const item = fileQueue.find((f) => f.id === fileId);
  const state = uploadStates.get(fileId);

  if (!item || !state || state.isPaused) return;

  // Fill up to max concurrent
  while (
    state.inFlight.size < state.maxConcurrent &&
    state.pending.length > 0
  ) {
    const chunkId = state.pending.shift();
    sendChunk(fileId, chunkId);
  }

  // Check if complete
  if (state.pending.length === 0 && state.inFlight.size === 0) {
    item.status = "complete";
    item.progress = 100;
    updateFileDisplay(fileId);
    updateUploadButton();
    checkAllComplete();
  }
}

/**
 * Send a single chunk via XHR
 * @param {string} fileId
 * @param {number} chunkId
 */
function sendChunk(fileId, chunkId) {
  const item = fileQueue.find((f) => f.id === fileId);
  const state = uploadStates.get(fileId);

  if (!item || !state || state.isPaused) return;

  const start = chunkId * state.chunkSize;
  const end = Math.min(start + state.chunkSize, item.size);
  const chunk = item.file.slice(start, end);
  const totalChunks = Math.ceil(item.size / state.chunkSize);

  state.inFlight.add(chunkId);

  const xhr = new XMLHttpRequest();
  state.xhrs.set(chunkId, xhr);

  // Get upload path from current URL
  const uploadPath = window.location.pathname;

  xhr.open("POST", uploadPath, true);
  xhr.setRequestHeader("X-File-Name", encodeURIComponent(item.name));
  xhr.setRequestHeader("X-Upload-Session", item.sessionId);
  xhr.setRequestHeader("X-Chunk-Id", chunkId.toString());
  xhr.setRequestHeader("X-Chunk-Total", totalChunks.toString());
  xhr.setRequestHeader("X-Upload-Offset", start.toString());
  xhr.setRequestHeader("X-Upload-Total", item.size.toString());
  xhr.setRequestHeader("Content-Type", "application/octet-stream");

  // Track upload progress for this chunk
  let lastLoaded = 0;
  let lastTime = Date.now();

  xhr.upload.onprogress = (e) => {
    if (!e.lengthComputable) return;

    const now = Date.now();
    const timeDelta = (now - lastTime) / 1000;
    const bytesDelta = e.loaded - lastLoaded;

    if (timeDelta > 0) {
      item.speed = bytesDelta / timeDelta;
    }

    lastLoaded = e.loaded;
    lastTime = now;

    // Update overall progress
    const chunkProgress = e.loaded;
    const completedChunks =
      totalChunks - state.pending.length - state.inFlight.size;
    const completedBytes = completedChunks * state.chunkSize + chunkProgress;
    item.progress = calculateProgress(completedBytes, item.size);

    updateFileDisplay(fileId);
  };

  xhr.onload = () => {
    state.inFlight.delete(chunkId);
    state.xhrs.delete(chunkId);

    if (xhr.status >= 200 && xhr.status < 300) {
      state.completedBytes += end - start;
      scheduleChunks(fileId);
    } else {
      handleChunkError(fileId, chunkId, `Server error: ${xhr.status}`);
    }
  };

  xhr.onerror = () => {
    state.inFlight.delete(chunkId);
    state.xhrs.delete(chunkId);
    handleChunkError(fileId, chunkId, "Network error");
  };

  xhr.onabort = () => {
    state.inFlight.delete(chunkId);
    state.xhrs.delete(chunkId);
  };

  xhr.send(chunk);
}

/**
 * Handle chunk upload error with retry logic
 * @param {string} fileId
 * @param {number} chunkId
 * @param {string} errorMsg
 */
function handleChunkError(fileId, chunkId, errorMsg) {
  const item = fileQueue.find((f) => f.id === fileId);
  const state = uploadStates.get(fileId);

  if (!item || !state) return;

  // Track retry count per chunk
  if (!state.retries) state.retries = new Map();
  const retryCount = (state.retries.get(chunkId) || 0) + 1;
  state.retries.set(chunkId, retryCount);

  if (retryCount < 3) {
    // Retry with exponential backoff
    const delay = Math.min(1000 * Math.pow(2, retryCount - 1), 10000);
    setTimeout(() => {
      if (!state.isPaused && item.status === "uploading") {
        state.pending.unshift(chunkId);
        scheduleChunks(fileId);
      }
    }, delay);
  } else {
    // Max retries exceeded
    item.status = "error";
    item.error = errorMsg;
    state.isPaused = true;

    // Abort remaining uploads
    for (const xhr of state.xhrs.values()) {
      xhr.abort();
    }

    updateFileDisplay(fileId);
    updateUploadButton();
  }
}

/**
 * Check if all uploads are complete
 */
function checkAllComplete() {
  const allComplete = fileQueue.every(
    (f) => f.status === "complete" || f.status === "error",
  );
  const hasErrors = fileQueue.some((f) => f.status === "error");

  if (allComplete) {
    if (hasErrors) {
      updateStatus("Upload completed with errors");
    } else {
      updateStatus("All uploads complete");
    }
  }
}

// ============================================================================
// WebSocket Client
// ============================================================================

/**
 * Connect to WebSocket for progress updates
 */
function connectWebSocket() {
  if (ws && ws.readyState === WebSocket.OPEN) return;

  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsUrl = `${protocol}//${window.location.host}/ws/progress`;

  try {
    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      connectionState.websocket = "connected";
      reconnectAttempts = 0;
      updateConnectionStatus();
      stopHealthPolling();
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        handleWebSocketMessage(data);
      } catch (e) {
        console.error("Failed to parse WebSocket message:", e);
      }
    };

    ws.onclose = () => {
      connectionState.websocket = "disconnected";
      ws = null;
      updateConnectionStatus();
      scheduleReconnect();
    };

    ws.onerror = () => {
      connectionState.websocket = "disconnected";
      updateConnectionStatus();
    };
  } catch (e) {
    console.error("WebSocket connection failed:", e);
    connectionState.websocket = "disconnected";
    updateConnectionStatus();
    scheduleReconnect();
  }
}

/**
 * Schedule WebSocket reconnection with exponential backoff
 */
function scheduleReconnect() {
  if (reconnectTimeout) return;

  connectionState.websocket = "reconnecting";
  updateConnectionStatus();

  // Exponential backoff: min(1000 * 2^n, 30000)
  const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), 30000);
  reconnectAttempts++;

  reconnectTimeout = setTimeout(() => {
    reconnectTimeout = null;
    connectWebSocket();
  }, delay);

  // Start health polling as fallback
  startHealthPolling();
}

/**
 * Handle incoming WebSocket message
 * @param {Object} data
 */
function handleWebSocketMessage(data) {
  if (data.type === "progress" && data.transfers) {
    for (const transfer of data.transfers) {
      updateTransferFromServer(transfer);
    }
  }
}

/**
 * Update file transfer state from server progress data
 * @param {Object} transfer
 */
function updateTransferFromServer(transfer) {
  // Match by filename
  const item = fileQueue.find((f) => f.name === transfer.filename);
  if (!item) return;

  if (transfer.progress !== undefined) {
    item.progress = transfer.progress;
  }
  if (transfer.speed !== undefined) {
    item.speed = transfer.speed;
  }
  if (transfer.status !== undefined) {
    item.status = transfer.status;
  }

  updateFileDisplay(item.id);
}

// ============================================================================
// Health Polling Fallback
// ============================================================================

/**
 * Start polling /health endpoint
 */
function startHealthPolling() {
  if (healthPollInterval) return;

  healthPollInterval = setInterval(async () => {
    try {
      const response = await fetch("/health");
      if (response.ok) {
        connectionState.server = "online";
        connectionState.lastPing = Date.now();
      } else {
        connectionState.server = "offline";
      }
    } catch (e) {
      connectionState.server = "offline";
    }

    // Also check pause state
    try {
      const pauseResponse = await fetch("/pause-state");
      if (pauseResponse.ok) {
        const data = await pauseResponse.json();
        if (data.paused) {
          connectionState.server = "paused";
          updateStatus("Server paused");
        }
      }
    } catch (e) {
      // Ignore pause state errors
    }

    updateConnectionStatus();
  }, 5000);
}

/**
 * Stop health polling
 */
function stopHealthPolling() {
  if (healthPollInterval) {
    clearInterval(healthPollInterval);
    healthPollInterval = null;
  }
}

// ============================================================================
// UI Update Functions
// ============================================================================

/**
 * Update connection status indicator
 */
function updateConnectionStatus() {
  if (!connectionStatus || !connectionText) return;

  const indicator = connectionStatus.querySelector(".status-dot");

  switch (connectionState.websocket) {
    case "connected":
      if (indicator) indicator.className = "status-dot connected";
      connectionText.textContent = "Connected";
      break;
    case "reconnecting":
      if (indicator) indicator.className = "status-dot reconnecting";
      connectionText.textContent = "Reconnecting...";
      break;
    case "disconnected":
    default:
      if (indicator) indicator.className = "status-dot disconnected";
      connectionText.textContent = "Disconnected";
      break;
  }

  // Update based on server state if WebSocket is down
  if (connectionState.websocket !== "connected") {
    switch (connectionState.server) {
      case "online":
        connectionText.textContent = "Online (polling)";
        break;
      case "paused":
        connectionText.textContent = "Server Paused";
        if (indicator) indicator.className = "status-dot paused";
        break;
      case "offline":
        connectionText.textContent = "Offline";
        break;
    }
  }
}

/**
 * Update status text in footer
 * @param {string} message
 */
function updateStatus(message) {
  if (statusText) {
    statusText.textContent = message;
  }
}

/**
 * Show notification message
 * @param {string} message
 * @param {'info'|'warning'|'error'|'success'} type
 */
function showNotification(message, type = "info") {
  // Create notification element
  const notification = document.createElement("div");
  notification.className = `notification notification-${type}`;
  notification.textContent = message;

  // Find or create notification container
  let container = document.getElementById("notification-container");
  if (!container) {
    container = document.createElement("div");
    container.id = "notification-container";
    container.className = "notification-container";
    document.body.appendChild(container);
  }

  container.appendChild(notification);

  // Auto-remove after 3 seconds
  setTimeout(() => {
    notification.classList.add("fade-out");
    setTimeout(() => {
      notification.remove();
    }, 300);
  }, 3000);
}

// ============================================================================
// Event Listeners
// ============================================================================

/**
 * Initialize drag-and-drop event listeners
 */
function initDragDrop() {
  if (!uploadZone) return;

  // Prevent default drag behaviors on document
  ["dragenter", "dragover", "dragleave", "drop"].forEach((eventName) => {
    document.addEventListener(eventName, (e) => {
      e.preventDefault();
      e.stopPropagation();
    });
  });

  // Highlight drop zone on drag over
  ["dragenter", "dragover"].forEach((eventName) => {
    uploadZone.addEventListener(eventName, () => {
      uploadZone.classList.add("drag-over");
    });
  });

  // Remove highlight on drag leave or drop
  ["dragleave", "drop"].forEach((eventName) => {
    uploadZone.addEventListener(eventName, () => {
      uploadZone.classList.remove("drag-over");
    });
  });

  // Handle dropped files
  uploadZone.addEventListener("drop", (e) => {
    const files = e.dataTransfer.files;
    if (files.length > 0) {
      addFiles(files);
    }
  });

  // Click to open file picker
  uploadZone.addEventListener("click", () => {
    if (fileInput) {
      fileInput.click();
    }
  });
}

/**
 * Initialize file input event listener
 */
function initFileInput() {
  if (!fileInput) return;

  fileInput.addEventListener("change", (e) => {
    const files = e.target.files;
    if (files.length > 0) {
      addFiles(files);
    }
    // Reset input so same file can be selected again
    fileInput.value = "";
  });
}

/**
 * Initialize upload button event listener
 */
function initUploadButton() {
  if (!uploadBtn) return;

  uploadBtn.addEventListener("click", () => {
    const hasQueuedFiles = fileQueue.some((f) => f.status === "queued");
    if (hasQueuedFiles) {
      startUpload();
    }
  });
}

/**
 * Initialize keyboard shortcuts
 */
function initKeyboardShortcuts() {
  document.addEventListener("keydown", (e) => {
    // Space to pause/resume
    if (e.code === "Space" && !e.target.matches("input, textarea")) {
      e.preventDefault();
      const uploadingFile = fileQueue.find((f) => f.status === "uploading");
      const pausedFile = fileQueue.find((f) => f.status === "paused");

      if (uploadingFile) {
        pauseUpload(uploadingFile.id);
      } else if (pausedFile) {
        resumeUpload(pausedFile.id);
      }
    }

    // Escape to cancel
    if (e.code === "Escape") {
      const activeFile = fileQueue.find(
        (f) => f.status === "uploading" || f.status === "paused",
      );
      if (activeFile) {
        cancelUpload(activeFile.id);
      }
    }
  });
}

/**
 * Fetch user IP address
 */
async function fetchUserIp() {
  if (!userIp) return;

  try {
    const response = await fetch("/client-ip");
    if (response.ok) {
      const data = await response.json();
      userIp.textContent = data.ip || "Unknown";
    }
  } catch (e) {
    userIp.textContent = "Unknown";
  }
}

// ============================================================================
// Initialization
// ============================================================================

/**
 * Initialize the application
 */
function init() {
  // Initialize event listeners
  initDragDrop();
  initFileInput();
  initUploadButton();
  initKeyboardShortcuts();

  // Initialize UI state
  renderFileQueue();
  updateUploadButton();
  updateConnectionStatus();
  updateStatus("Ready");

  // Connect to WebSocket
  connectWebSocket();

  // Fetch user IP
  fetchUserIp();
}

// Start when DOM is ready
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
