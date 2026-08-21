// Noctifab Dark Factory — Live Mission Control Client

document.addEventListener('DOMContentLoaded', () => {
  // DOM Elements
  const sseStatus = document.getElementById('sse-status');
  const storyStatus = document.getElementById('story-status');
  const buildHealth = document.getElementById('build-health');
  const totalTokens = document.getElementById('total-tokens');
  const elapsedTime = document.getElementById('elapsed-time');
  const activeStoryName = document.getElementById('active-story-name');
  const agentsGrid = document.getElementById('agents-grid');
  const activeAgentsCount = document.getElementById('active-agents-count');
  const dagContainer = document.getElementById('dag-container');
  const diffContent = document.getElementById('diff-content');
  const logStream = document.getElementById('log-stream');
  const btnPause = document.getElementById('btn-pause');
  const btnResume = document.getElementById('btn-resume');
  const btnClearLogs = document.getElementById('btn-clear-logs');
  const orderForm = document.getElementById('order-form');
  const orderType = document.getElementById('order-type');
  const orderInput = document.getElementById('order-input');
  const actionCount = document.getElementById('action-count');

  // Clarification banner DOM elements
  const clarificationBanner = document.getElementById('clarification-banner');
  const clarificationTaskId = document.getElementById('clarification-task-id');
  const clarificationQuestionText = document.getElementById('clarification-question-text');
  const clarificationForm = document.getElementById('clarification-form');
  const clarificationInput = document.getElementById('clarification-input');
  let activeClarificationId = null;

  // Terminal console DOM elements
  const terminalDrawer = document.getElementById('terminal-drawer');
  const terminalToggle = document.getElementById('terminal-toggle');
  const terminalChevron = document.getElementById('terminal-chevron');
  const terminalOutput = document.getElementById('terminal-output');
  const terminalAutoscroll = document.getElementById('terminal-autoscroll');
  const terminalClear = document.getElementById('terminal-clear');

  // Filter chips
  const countAll = document.getElementById('count-all');
  const countActive = document.getElementById('count-active');
  const countFailed = document.getElementById('count-failed');
  const countDone = document.getElementById('count-done');
  const filterChips = document.querySelectorAll('.chip');
  let currentFilter = 'all';

  // Tabs
  const tabDiffBtn = document.getElementById('tab-diff-btn');
  const tabLogsBtn = document.getElementById('tab-logs-btn');
  const viewDiff = document.getElementById('view-diff');
  const viewLogs = document.getElementById('view-logs');

  let currentState = null;
  let eventLogEntries = [];
  let startTime = Date.now();

  // 1. Initial State Fetch
  async function fetchState() {
    try {
      const res = await fetch('/api/v1/state');
      if (res.ok) {
        currentState = await res.json();
        updateUI(currentState);
      }
    } catch (e) {
      console.warn('Initial state fetch failed, waiting for live SSE stream...', e);
    }
  }

  // 2. Connect to Server-Sent Events (SSE)
  function connectSSE() {
    const sse = new EventSource('/api/v1/events');

    sse.onopen = () => {
      if (sseStatus) {
        sseStatus.textContent = 'Live Stream Connected';
        sseStatus.parentElement.style.color = 'var(--accent-green)';
      }
    };

    sse.addEventListener('TASK_STATE_CHANGED', (e) => {
      try {
        const payload = JSON.parse(e.data);
        if (payload.tasks && currentState) {
          currentState.tasks = payload.tasks;
          renderTasks(currentState.tasks);
          updateTaskCounts(currentState.tasks);
          updatePipelinePhase(currentState);
        }
        appendLogEntry('TASK', `Task state updated: ${payload.task_id || 'batch'}`, 'system');
        appendTerminalLine(`[TASK STATE] ${payload.task_id || 'batch'}: ${payload.status || 'updated'}`);
      } catch (err) {
        console.error('Error in TASK_STATE_CHANGED event:', err);
      }
    });

    sse.addEventListener('DIFF_CHUNK_APPENDED', (e) => {
      try {
        const payload = JSON.parse(e.data);
        if (payload.diff) {
          renderDiff(payload.diff);
          appendTerminalLine(`[DIFF UPDATED] ${payload.file || 'patch'} (${payload.diff.length} bytes)`);
        }
      } catch (err) {
        console.error('Error in DIFF_CHUNK_APPENDED event:', err);
      }
    });

    sse.addEventListener('TOKEN_METRICS', (e) => {
      try {
        const payload = JSON.parse(e.data);
        if (payload.total_tokens !== undefined) totalTokens.textContent = Number(payload.total_tokens).toLocaleString();
      } catch (err) {
        console.error('Error in TOKEN_METRICS event:', err);
      }
    });

    sse.addEventListener('CONSENSUS_VOTE', (e) => {
      try {
        const payload = JSON.parse(e.data);
        appendLogEntry('CONSENSUS', `Vote recorded for task ${payload.task_id}: ${payload.vote || 'PASS'}`, 'tool-success');
        appendTerminalLine(`[CONSENSUS VOTE] Task: ${payload.task_id} => Vote: ${payload.vote || 'PASS'}`);
      } catch (err) {
        console.error('Error in CONSENSUS_VOTE event:', err);
      }
    });

    sse.addEventListener('SYSTEM_LOG', (e) => {
      try {
        const payload = JSON.parse(e.data);
        appendLogEntry(payload.level || 'LOG', payload.message || JSON.stringify(payload), 'system');
        appendTerminalLine(`[${payload.level || 'LOG'}] ${payload.message || JSON.stringify(payload)}`);
      } catch (err) {
        console.error('Error in SYSTEM_LOG event:', err);
      }
    });

    sse.onmessage = (e) => {
      try {
        const state = JSON.parse(e.data);
        if (state && typeof state === 'object') {
          currentState = state;
          updateUI(state);
        }
      } catch (err) {
        // Ping or keepalive comment
      }
    };

    sse.onerror = () => {
      if (sseStatus) {
        sseStatus.textContent = 'Reconnecting...';
        sseStatus.parentElement.style.color = 'var(--accent-yellow)';
      }
    };
  }

  // 3. UI Updates
  function updateUI(state) {
    if (!state) return;

    // Header stats
    if (state.story_status) {
      if (state.story_status === 'IDLE') {
        storyStatus.textContent = 'STANDBY 🟢';
        storyStatus.className = 'badge badge-info';
      } else if (state.story_status === 'SUCCESS') {
        storyStatus.textContent = 'SUCCESS ✅';
        storyStatus.className = 'badge badge-success';
      } else if (state.story_status === 'PAUSED') {
        storyStatus.textContent = 'PAUSED ⏸';
        storyStatus.className = 'badge badge-warning';
      } else if (state.story_status === 'RUNNING') {
        storyStatus.textContent = 'RUNNING ⚡';
        storyStatus.className = 'badge badge-success';
      } else {
        storyStatus.textContent = state.story_status;
        storyStatus.className = 'badge badge-danger';
      }

      if (state.story_status === 'PAUSED') {
        btnPause.style.display = 'none';
        btnResume.style.display = 'inline-block';
      } else {
        btnPause.style.display = 'inline-block';
        btnResume.style.display = 'none';
      }
    }

    if (state.build_status) {
      buildHealth.textContent = state.build_status === 'FAILING' ? 'FAILING ❌' : 'PASSING ✅';
      buildHealth.className = 'badge ' + (state.build_status === 'FAILING' ? 'badge-danger' : 'badge-success');
    }

    if (state.metadata) {
      if (state.metadata.feature_name) activeStoryName.textContent = state.metadata.feature_name;
      if (state.metadata.total_tokens_used !== undefined) totalTokens.textContent = Number(state.metadata.total_tokens_used).toLocaleString();
    }

    updatePipelinePhase(state);
    renderClarifications(state.clarifications || []);
    renderAgents(state.active_agents || []);
    renderTasks(state.tasks || []);
    updateTaskCounts(state.tasks || []);
    renderActions(state.last_actions || []);
  }

  // 4. Pipeline Phase Stepper
  function updatePipelinePhase(state) {
    const steps = ['roadmap', 'planner', 'generator', 'tester', 'consensus', 'vcs'];
    steps.forEach(s => {
      const el = document.getElementById('step-' + s);
      if (el) el.className = 'step';
    });

    let currentStep = 'roadmap';
    const tasks = state.tasks || [];
    const hasRunning = tasks.some(t => t.status === 'IN_PROGRESS');
    const allDone = tasks.length > 0 && tasks.every(t => t.status === 'SUCCESS');

    if (tasks.length === 0) {
      currentStep = 'planner';
    } else if (hasRunning) {
      const activeTask = tasks.find(t => t.status === 'IN_PROGRESS');
      if (activeTask && activeTask.assigned_to && activeTask.assigned_to.toLowerCase().includes('test')) {
        currentStep = 'tester';
      } else {
        currentStep = 'generator';
      }
    } else if (allDone) {
      currentStep = state.metadata && state.metadata.pr_url ? 'vcs' : 'consensus';
    }

    const activeIndex = steps.indexOf(currentStep);
    steps.forEach((s, idx) => {
      const el = document.getElementById('step-' + s);
      if (el) {
        if (idx < activeIndex) el.className = 'step completed';
        else if (idx === activeIndex) el.className = 'step active';
      }
    });
  }

  // 5. Active Agent Workers Grid
  function renderAgents(agents) {
    if (activeAgentsCount) {
      activeAgentsCount.textContent = `${agents.length} active goroutine${agents.length === 1 ? '' : 's'}`;
    }

    if (!agents || agents.length === 0) {
      agentsGrid.innerHTML = '<div class="empty-agent-state">No agent workers currently active (system idle or awaiting trigger)</div>';
      return;
    }

    agentsGrid.innerHTML = agents.map(ag => {
      const roleIcons = {
        'planner': '🧠 PLANNER',
        'generator': '⚡ GENERATOR',
        'tester': '🧪 TESTER',
        'qa': '🛡️ QA CONSENSUS'
      };
      const roleLabel = roleIcons[ag.role ? ag.role.toLowerCase() : ''] || (ag.role || 'AGENT');
      return `
        <div class="agent-card ${ag.status === 'WORKING' ? 'working' : ''}">
          <div class="agent-card-header">
            <span class="agent-role">${escapeHtml(roleLabel)}</span>
            <span class="badge ${ag.status === 'WORKING' ? 'badge-warning' : 'badge-success'}">${ag.status || 'ACTIVE'}</span>
          </div>
          <div class="agent-tool-call">
            ${ag.task_id ? `Task: <strong>${escapeHtml(ag.task_id)}</strong>` : 'Analyzing requirements...'}
          </div>
          <div class="agent-goal">
            Worker ID: <code>${escapeHtml(ag.id || ag.name)}</code>
          </div>
        </div>
      `;
    }).join('');
  }

  // 6. Task DAG Rendering with Filter
  function renderTasks(tasks) {
    if (!tasks || tasks.length === 0) {
      dagContainer.innerHTML = '<div class="empty-state">No tasks scheduled in DAG yet.</div>';
      return;
    }

    const filtered = tasks.filter(t => {
      if (currentFilter === 'active') return t.status === 'IN_PROGRESS';
      if (currentFilter === 'failed') return t.status === 'FAILED' || t.status === 'CONFLICT_FAILED';
      if (currentFilter === 'done') return t.status === 'SUCCESS';
      return true;
    });

    if (filtered.length === 0) {
      dagContainer.innerHTML = `<div class="empty-state">No tasks match filter "${currentFilter}".</div>`;
      return;
    }

    dagContainer.innerHTML = filtered.map(t => {
      const statusClass = (t.status || 'PENDING').toLowerCase().replace('_', '-');
      const progress = t.progress || 0;
      const deps = t.depends_on && t.depends_on.length > 0 ? t.depends_on.join(', ') : null;
      const targetFiles = t.target_files && t.target_files.length > 0 ? t.target_files.join(', ') : null;
      const steering = t.user_directives && t.user_directives.length > 0 ? t.user_directives[t.user_directives.length - 1] : null;

      return `
        <div class="task-node ${statusClass}">
          <div class="task-top-row">
            <div>
              <span class="task-title">${escapeHtml(t.title || t.id)}</span>
              ${t.change_type ? `<span class="task-change-type">${escapeHtml(t.change_type)}</span>` : ''}
            </div>
            <span class="badge ${t.status === 'SUCCESS' ? 'badge-success' : t.status === 'IN_PROGRESS' ? 'badge-warning' : 'badge-danger'}">
              ${t.status || 'PENDING'} (${progress}%)
            </span>
          </div>

          <div class="progress-bar-container">
            <div class="progress-bar-fill" style="width: ${progress}%;"></div>
          </div>

          <div class="task-meta-row">
            ${deps ? `<div class="meta-item">🔗 Depends on: <strong>${escapeHtml(deps)}</strong></div>` : ''}
            ${targetFiles ? `<div class="meta-item">📁 Target: <code>${escapeHtml(targetFiles)}</code></div>` : ''}
            ${t.retries > 0 ? `<div class="meta-item">⚠️ Retries: ${t.retries}</div>` : ''}
          </div>

          ${steering ? `<div class="steering-tag">🎯 User Steering: "${escapeHtml(steering)}"</div>` : ''}

          ${(t.status === 'FAILED' || t.status === 'CONFLICT_FAILED') && t.failure_log ? `
            <div class="failure-excerpt">Error: ${escapeHtml(t.failure_log.slice(0, 150))}...</div>
          ` : ''}
        </div>
      `;
    }).join('');
  }

  function updateTaskCounts(tasks) {
    if (!tasks) return;
    countAll.textContent = tasks.length;
    countActive.textContent = tasks.filter(t => t.status === 'IN_PROGRESS').length;
    countFailed.textContent = tasks.filter(t => t.status === 'FAILED' || t.status === 'CONFLICT_FAILED').length;
    countDone.textContent = tasks.filter(t => t.status === 'SUCCESS').length;
  }

  // 7. Completed Actions & Live Feed
  function renderActions(actions) {
    if (!actions) return;
    actionCount.textContent = actions.length;
    actions.forEach(act => {
      const ts = act.timestamp ? new Date(act.timestamp).toLocaleTimeString() : 'now';
      const cssClass = act.success ? 'tool-success' : 'tool-fail';
      appendLogEntry(act.tool || 'ACTION', `Result: ${act.result || act.reasoning || 'Executed'}`, cssClass, ts);
    });
  }

  function appendLogEntry(tag, message, cssClass = 'system', timestamp = null) {
    const timeStr = timestamp || new Date().toLocaleTimeString();
    const entry = document.createElement('div');
    entry.className = `log-entry ${cssClass}`;
    entry.innerHTML = `<span>[${timeStr}]</span> <strong>[${escapeHtml(tag)}]</strong> <span>${escapeHtml(message)}</span>`;
    logStream.appendChild(entry);
    logStream.scrollTop = logStream.scrollHeight;
  }

  // 8. Diff Viewer Syntax Highlighting
  function renderDiff(diffText) {
    if (!diffText) return;
    const lines = diffText.split('\n');
    diffContent.innerHTML = lines.map(line => {
      const escaped = escapeHtml(line);
      if (line.startsWith('+') && !line.startsWith('+++')) return `<span class="diff-line-add">${escaped}</span>`;
      if (line.startsWith('-') && !line.startsWith('---')) return `<span class="diff-line-del">${escaped}</span>`;
      if (line.startsWith('@@')) return `<span class="diff-line-hunk">${escaped}</span>`;
      return `<span>${escaped}</span>`;
    }).join('\n');
  }

  // 9. Tab Switching
  tabDiffBtn.addEventListener('click', () => {
    tabDiffBtn.classList.add('active');
    tabLogsBtn.classList.remove('active');
    viewDiff.classList.add('active');
    viewLogs.classList.remove('active');
  });

  tabLogsBtn.addEventListener('click', () => {
    tabLogsBtn.classList.add('active');
    tabDiffBtn.classList.remove('active');
    viewLogs.classList.add('active');
    viewDiff.classList.remove('active');
  });

  btnClearLogs.addEventListener('click', () => {
    logStream.innerHTML = '';
  });

  // 10. Filter Chips
  filterChips.forEach(chip => {
    chip.addEventListener('click', () => {
      filterChips.forEach(c => c.classList.remove('active'));
      chip.classList.add('active');
      currentFilter = chip.getAttribute('data-filter');
      if (currentState) renderTasks(currentState.tasks || []);
    });
  });

  // 11. Pause / Resume Handlers
  btnPause.addEventListener('click', async () => {
    await fetch('/api/v1/pause', { method: 'POST' });
    storyStatus.textContent = 'PAUSED';
    btnPause.style.display = 'none';
    btnResume.style.display = 'inline-block';
    appendLogEntry('SYSTEM', 'Execution paused by user via web control', 'system');
  });

  btnResume.addEventListener('click', async () => {
    await fetch('/api/v1/resume', { method: 'POST' });
    storyStatus.textContent = 'RUNNING';
    btnResume.style.display = 'none';
    btnPause.style.display = 'inline-block';
    appendLogEntry('SYSTEM', 'Execution resumed by user via web control', 'system');
  });

  // 12. Form Order / Steering Submit
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
        appendLogEntry(orderType.value.toUpperCase(), text, 'steer');
        orderInput.value = '';
        orderInput.placeholder = 'Sent! Enter next order or steering directive...';
      }
    } catch (err) {
      console.error('Failed to submit order:', err);
    }
  });

  // 13. Global Keyboard Shortcuts
  document.addEventListener('keydown', (e) => {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        orderForm.requestSubmit();
      }
      return;
    }
    if (e.key === 'p' || e.key === 'P') {
      if (btnPause.style.display !== 'none') btnPause.click();
      else if (btnResume.style.display !== 'none') btnResume.click();
    } else if (e.key === 's' || e.key === 'S') {
      orderType.value = 'steer';
      orderInput.focus();
      e.preventDefault();
    } else if (e.key === 'o' || e.key === 'O') {
      orderType.value = 'order';
      orderInput.focus();
      e.preventDefault();
    }
  });

  // 14. Clarifications Rendering & Submission
  function renderClarifications(clarifications) {
    if (!clarificationBanner) return;
    const pending = (clarifications || []).filter(c => !c.resolved);
    if (pending.length === 0) {
      clarificationBanner.style.display = 'none';
      activeClarificationId = null;
      return;
    }
    const current = pending[0];
    activeClarificationId = current.id || current.question;
    if (clarificationTaskId) {
      clarificationTaskId.textContent = current.task_id || current.id || 'GLOBAL';
    }
    if (clarificationQuestionText) {
      clarificationQuestionText.textContent = current.question;
    }
    clarificationBanner.style.display = 'flex';
  }

  if (clarificationForm) {
    clarificationForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const answer = clarificationInput.value.trim();
      if (!answer || !activeClarificationId) return;

      try {
        const res = await fetch(`/api/v1/clarifications/${encodeURIComponent(activeClarificationId)}/resolve`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ answer })
        });
        if (res.ok) {
          appendLogEntry('CLARIFY', `Resolved clarification [${activeClarificationId}]: ${answer}`, 'steer');
          appendTerminalLine(`[CLARIFICATION RESOLVED] ID: ${activeClarificationId} => Answer: ${answer}`);
          clarificationInput.value = '';
          clarificationBanner.style.display = 'none';
          activeClarificationId = null;
        }
      } catch (err) {
        console.error('Failed to submit clarification answer:', err);
      }
    });
  }

  // 15. Streaming Console Terminal Helper & Controls
  function appendTerminalLine(text) {
    if (!terminalOutput) return;
    const now = new Date();
    const timeStr = now.toTimeString().split(' ')[0];
    terminalOutput.textContent += `\n[${timeStr}] ${text}`;
    if (terminalAutoscroll && terminalAutoscroll.checked) {
      terminalOutput.scrollTop = terminalOutput.scrollHeight;
    }
  }

  if (terminalToggle) {
    terminalToggle.addEventListener('click', (e) => {
      if (e.target.closest('.terminal-controls')) return;
      terminalDrawer.classList.toggle('collapsed');
      if (terminalChevron) {
        terminalChevron.textContent = terminalDrawer.classList.contains('collapsed') ? '▲' : '▼';
      }
    });
  }

  if (terminalClear) {
    terminalClear.addEventListener('click', (e) => {
      e.stopPropagation();
      if (terminalOutput) {
        terminalOutput.textContent = '// Terminal cleared. Listening for stream...';
      }
    });
  }

  // 16. Spec Studio (Visual Web Spec Editor) Controller
  const btnToggleSpec = document.getElementById('btn-toggle-spec');
  const specStudioView = document.getElementById('spec-studio-view');
  const mainSplit = document.querySelector('.main-split');
  const agentsSection = document.querySelector('.agents-section');
  const specEditorTextarea = document.getElementById('spec-editor-textarea');
  const specDiffContent = document.getElementById('spec-diff-content');
  const specRefineForm = document.getElementById('spec-refine-form');
  const specFeedbackInput = document.getElementById('spec-feedback-input');
  const btnSubmitRefine = document.getElementById('btn-submit-refine');
  const btnApproveSpec = document.getElementById('btn-approve-spec');
  const btnAuditSpec = document.getElementById('btn-audit-spec');

  let isSpecStudioActive = false;

  if (btnToggleSpec) {
    btnToggleSpec.addEventListener('click', () => {
      isSpecStudioActive = !isSpecStudioActive;
      if (isSpecStudioActive) {
        btnToggleSpec.textContent = '📊 Dashboard View';
        btnToggleSpec.classList.remove('btn-warning');
        btnToggleSpec.classList.add('btn-primary');
        if (mainSplit) mainSplit.style.display = 'none';
        if (agentsSection) agentsSection.style.display = 'none';
        if (specStudioView) specStudioView.style.display = 'grid';
        fetchSpec();
      } else {
        btnToggleSpec.textContent = '📄 Spec Studio';
        btnToggleSpec.classList.remove('btn-primary');
        btnToggleSpec.classList.add('btn-warning');
        if (mainSplit) mainSplit.style.display = 'grid';
        if (agentsSection) agentsSection.style.display = 'block';
        if (specStudioView) specStudioView.style.display = 'none';
      }
    });
  }

  const btnSpecUndo = document.getElementById('btn-spec-undo');
  const btnSpecRedo = document.getElementById('btn-spec-redo');
  const specTimelinePills = document.getElementById('spec-timeline-pills');
  const specStatusBadge = document.getElementById('spec-status-badge');

  let currentActiveVer = 1;
  let availableVersions = [1];

  async function fetchSpec() {
    try {
      const res = await fetch('/api/v1/spec');
      if (res.ok) {
        const data = await res.json();
        updateSpecView(data);
      }
    } catch (err) {
      console.error('Failed to fetch specification:', err);
    }
  }

  function updateSpecView(data) {
    if (specEditorTextarea) specEditorTextarea.value = data.content || '';
    if (data.latest_diff && specDiffContent) {
      specDiffContent.textContent = data.latest_diff;
    }
    if (data.active_version) currentActiveVer = data.active_version;
    if (data.available_versions) availableVersions = data.available_versions;

    if (specStatusBadge) {
      specStatusBadge.textContent = `Revision ${currentActiveVer}`;
    }

    renderTimelinePills();

    if (data.model_roles) {
      const rolePm = document.getElementById('role-pm-model');
      const roleArch = document.getElementById('role-arch-model');
      const roleTest = document.getElementById('role-test-model');
      const roleQa = document.getElementById('role-qa-model');
      if (rolePm && data.model_roles['Product Manager']) rolePm.textContent = data.model_roles['Product Manager'];
      if (roleArch && data.model_roles['Systems Architect']) roleArch.textContent = data.model_roles['Systems Architect'];
      if (roleTest && data.model_roles['Test Architect']) roleTest.textContent = data.model_roles['Test Architect'];
      if (roleQa && data.model_roles['QA Specialist']) roleQa.textContent = data.model_roles['QA Specialist'];
    }
  }

  function renderTimelinePills() {
    if (!specTimelinePills) return;
    specTimelinePills.innerHTML = '';
    availableVersions.forEach((v) => {
      const pill = document.createElement('button');
      pill.className = `rev-pill ${v === currentActiveVer ? 'active' : ''}`;
      pill.setAttribute('data-version', v);
      pill.textContent = `● v${v} ${v === 1 ? '(Draft)' : ''}`;
      pill.addEventListener('click', () => checkoutVersion(v));
      specTimelinePills.appendChild(pill);
    });
  }

  async function checkoutVersion(v) {
    try {
      const res = await fetch('/api/v1/spec/checkout', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: v })
      });
      if (res.ok) {
        const data = await res.json();
        updateSpecView(data);
        appendLogEntry('SPEC', `Checked out specification revision v${v}`, 'steer');
      }
    } catch (err) {
      console.error('Checkout version failed:', err);
    }
  }

  if (btnSpecUndo) {
    btnSpecUndo.addEventListener('click', () => {
      if (currentActiveVer > 1) {
        checkoutVersion(currentActiveVer - 1);
      }
    });
  }

  if (btnSpecRedo) {
    btnSpecRedo.addEventListener('click', () => {
      if (currentActiveVer < availableVersions.length) {
        checkoutVersion(currentActiveVer + 1);
      }
    });
  }

  if (specRefineForm) {
    specRefineForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const prompt = specFeedbackInput.value.trim();
      if (!prompt) return;

      if (btnSubmitRefine) {
        btnSubmitRefine.disabled = true;
        btnSubmitRefine.textContent = '⏳ Multi-Model Pipeline Refining...';
      }

      try {
        const res = await fetch('/api/v1/spec/refine', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ prompt })
        });
        if (res.ok) {
          const data = await res.json();
          updateSpecView(data);
          specFeedbackInput.value = '';
          appendLogEntry('SPEC', `Refined specification with prompt: ${prompt}`, 'steer');
        }
      } catch (err) {
        console.error('Failed to refine spec:', err);
      } finally {
        if (btnSubmitRefine) {
          btnSubmitRefine.disabled = false;
          btnSubmitRefine.textContent = '🚀 Refine with Multi-Model Pipeline';
        }
      }
    });
  }

  if (btnApproveSpec) {
    btnApproveSpec.addEventListener('click', async () => {
      try {
        const res = await fetch('/api/v1/spec/approve', { method: 'POST' });
        if (res.ok) {
          alert('✔ Specification approved! Returning to mission control dashboard.');
          if (btnToggleSpec) btnToggleSpec.click();
        }
      } catch (err) {
        console.error('Failed to approve spec:', err);
      }
    });
  }

  if (btnAuditSpec) {
    btnAuditSpec.addEventListener('click', async () => {
      if (specFeedbackInput) {
        specFeedbackInput.value = 'Run a multi-model consistency audit to detect and resolve any section contradictions.';
        if (specRefineForm) specRefineForm.requestSubmit();
      }
    });
  }

  // Timer ticker
  setInterval(() => {
    const secs = Math.floor((Date.now() - startTime) / 1000);
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    elapsedTime.textContent = `${m > 0 ? m + 'm ' : ''}${s}s`;
  }, 1000);

  function escapeHtml(str) {
    return String(str || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  fetchState();
  connectSSE();
});
