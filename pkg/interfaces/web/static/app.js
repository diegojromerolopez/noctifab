// Noctifab Web Dashboard Client Application

document.addEventListener('DOMContentLoaded', () => {
  const storyStatus = document.getElementById('story-status');
  const totalTokens = document.getElementById('total-tokens');
  const totalCost = document.getElementById('total-cost');
  const activeStoryName = document.getElementById('active-story-name');
  const dagContainer = document.getElementById('dag-container');
  const diffContent = document.getElementById('diff-content');
  const btnPause = document.getElementById('btn-pause');
  const btnResume = document.getElementById('btn-resume');
  const orderForm = document.getElementById('order-form');
  const orderType = document.getElementById('order-type');
  const orderInput = document.getElementById('order-input');

  // 1. Initial State Fetch
  async function fetchState() {
    try {
      const res = await fetch('/api/v1/state');
      if (res.ok) {
        const state = await res.json();
        updateUI(state);
      }
    } catch (e) {
      console.warn('Initial state fetch failed, waiting for SSE stream...', e);
    }
  }

  // 2. Connect to Server-Sent Events (SSE)
  function connectSSE() {
    const sse = new EventSource('/api/v1/events');

    sse.addEventListener('TASK_STATE_CHANGED', (e) => {
      try {
        const payload = JSON.parse(e.data);
        renderTasks(payload.tasks || []);
      } catch (err) {
        console.error('Error handling TASK_STATE_CHANGED event:', err);
      }
    });

    sse.addEventListener('DIFF_CHUNK_APPENDED', (e) => {
      try {
        const payload = JSON.parse(e.data);
        if (payload.diff) {
          diffContent.textContent = payload.diff;
        }
      } catch (err) {
        console.error('Error handling DIFF_CHUNK_APPENDED event:', err);
      }
    });

    sse.addEventListener('TOKEN_METRICS', (e) => {
      try {
        const payload = JSON.parse(e.data);
        if (payload.total_tokens !== undefined) totalTokens.textContent = payload.total_tokens;
        if (payload.total_cost !== undefined) totalCost.textContent = payload.total_cost;
      } catch (err) {
        console.error('Error handling TOKEN_METRICS event:', err);
      }
    });

    sse.onmessage = (e) => {
      try {
        const state = JSON.parse(e.data);
        updateUI(state);
      } catch (err) {
        // Ping or unformatted message
      }
    };

    sse.onerror = () => {
      console.warn('SSE disconnected, browser will auto-reconnect...');
    };
  }

  function updateUI(state) {
    if (!state) return;
    if (state.story_status) {
      storyStatus.textContent = state.story_status;
      storyStatus.className = 'badge ' + (state.story_status === 'SUCCESS' ? 'badge-success' : state.story_status === 'PAUSED' ? 'badge-warning' : 'badge-danger');
      if (state.story_status === 'PAUSED') {
        btnPause.style.display = 'none';
        btnResume.style.display = 'inline-block';
      } else {
        btnPause.style.display = 'inline-block';
        btnResume.style.display = 'none';
      }
    }
    if (state.metadata) {
      if (state.metadata.feature_name) activeStoryName.textContent = state.metadata.feature_name;
      if (state.metadata.total_tokens_used !== undefined) totalTokens.textContent = state.metadata.total_tokens_used;
      if (state.metadata.total_cost_usd) totalCost.textContent = state.metadata.total_cost_usd;
    }
    if (state.tasks) {
      renderTasks(state.tasks);
    }
  }

  function renderTasks(tasks) {
    if (!tasks || tasks.length === 0) {
      dagContainer.innerHTML = '<div class="empty-state">No scheduled tasks in DAG</div>';
      return;
    }
    dagContainer.innerHTML = tasks.map(t => `
      <div class="task-node ${t.status ? t.status.toLowerCase().replace('_', '-') : 'pending'}">
        <div class="task-info">
          <span class="task-title">${escapeHtml(t.title || t.id)}</span>
          <span class="task-desc">${escapeHtml(t.description || '')}</span>
        </div>
        <span class="badge ${t.status === 'SUCCESS' ? 'badge-success' : t.status === 'IN_PROGRESS' ? 'badge-warning' : 'badge-danger'}">${t.status || 'PENDING'}</span>
      </div>
    `).join('');
  }

  function escapeHtml(str) {
    return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  // 3. Pause / Resume Buttons
  btnPause.addEventListener('click', async () => {
    await fetch('/api/v1/pause', { method: 'POST' });
    storyStatus.textContent = 'PAUSED';
    btnPause.style.display = 'none';
    btnResume.style.display = 'inline-block';
  });

  btnResume.addEventListener('click', async () => {
    await fetch('/api/v1/resume', { method: 'POST' });
    storyStatus.textContent = 'RUNNING';
    btnResume.style.display = 'none';
    btnPause.style.display = 'inline-block';
  });

  // 4. Form Submit for Orders and Steering
  orderForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const text = orderInput.value.trim();
    if (!text) return;

    const endpoint = orderType.value === 'steer' ? '/api/v1/steer' : '/api/v1/orders';
    const payload = orderType.value === 'steer' ? { directive: text } : { prompt: text };

    try {
      const res = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (res.ok) {
        orderInput.value = '';
        orderInput.placeholder = 'Sent! Enter another order or directive...';
      }
    } catch (err) {
      console.error('Failed to submit order:', err);
    }
  });

  fetchState();
  connectSSE();
});
