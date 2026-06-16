const API = "/api/v1";
const TERMINAL_KEY = "gmc.taskweb.terminal";
const POLL_MS = 5000;
const AGENTS = ["codex", "grok", "cursor-agent", "opencode"];
const TERMINALS = ["ghostty", "iterm", "terminal"];

const state = {
  project: null,
  workflow: null,
  tasks: [],
  details: {},
  selectedId: null,
  dragTaskId: null,
  polling: false,
  toastMessage: "",
  startTaskId: "",
  startTarget: "",
  attachTaskId: "",
  attachCli: "",
  removeTaskId: "",
  toastTimer: null,
  pollTimer: null,
};

const els = {};

function request(path, init = {}) {
  const headers = init.body ? { "Content-Type": "application/json" } : {};
  return fetch(API + path, { headers, ...init }).then(async (res) => {
    const body = await res.text();
    let data = null;
    if (body) {
      try {
        data = JSON.parse(body);
      } catch {
        data = { error: body };
      }
    }
    if (!res.ok) {
      const message = data && typeof data === "object" && "error" in data
        ? String(data.error)
        : res.statusText || "request failed";
      throw new Error(message);
    }
    return data;
  });
}

function create(tag, attrs = {}, children = []) {
  const el = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (value === false || value == null) continue;
    if (key === "class") el.className = value;
    else if (key === "text") el.textContent = value == null ? "" : String(value);
    else if (key === "dataset") Object.assign(el.dataset, value);
    else if (key === "style") Object.assign(el.style, value);
    else if (key.startsWith("on") && typeof value === "function") {
      el.addEventListener(key.slice(2).toLowerCase(), value);
    } else if (key in el) {
      el[key] = value;
    } else {
      el.setAttribute(key, value === true ? "" : String(value));
    }
  }
  for (const child of children) {
    if (child == null) continue;
    el.append(child.nodeType ? child : document.createTextNode(String(child)));
  }
  return el;
}

function labelFor(value) {
  if (value === "__add__") return "Add";
  if (value === "__done__" || value === "done") return "Done";
  return String(value || "")
    .replace(/[-_]+/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function columns() {
  if (!state.workflow) return [];
  return [
    { id: "__add__", label: "Add" },
    ...state.workflow.order.map((id) => ({ id, label: labelFor(id) })),
    { id: "__done__", label: "Done" },
  ];
}

function isDone(task) {
  return task.state === "done" || task.current_node === "done";
}

function isStarted(task) {
  return task.state !== "new";
}

function columnForTask(task) {
  if (task.state === "new") return "__add__";
  if (isDone(task)) return "__done__";
  return task.current_node || task.state;
}

function tasksForColumn(columnId) {
  return state.tasks.filter((task) => columnForTask(task) === columnId);
}

function workflowNode(columnId) {
  return state.workflow?.nodes?.[columnId] || {};
}

function dragTask() {
  return state.tasks.find((task) => task.id === state.dragTaskId) || null;
}

function canDrop(task, targetColumn) {
  if (!task) return false;
  if (targetColumn === "__add__") return false;
  if (isDone(task)) return false;
  if (columnForTask(task) === targetColumn) return false;
  return true;
}

function showToast(message) {
  state.toastMessage = message;
  if (state.toastTimer) clearTimeout(state.toastTimer);
  state.toastTimer = setTimeout(() => {
    state.toastMessage = "";
    renderToast();
  }, 3200);
  renderToast();
}

function savedTerminal() {
  const value = localStorage.getItem(TERMINAL_KEY)?.trim();
  return value && TERMINALS.includes(value) ? value : null;
}

function openAddDialog() {
  els.addSource.value = "";
  els.addDialog.showModal();
  requestAnimationFrame(() => els.addSource.focus());
}

function openStartDialog(task, targetColumn) {
  const node = workflowNode(targetColumn);
  state.startTaskId = task.id;
  state.startTarget = targetColumn;
  els.startAgent.value = node.agent && AGENTS.includes(node.agent) ? node.agent : "codex";
  els.startBase.value = "";
  els.startCommand.value = node.command || "";
  els.startTargetLabel.textContent = labelFor(targetColumn);
  els.startDialog.showModal();
}

function openAttachDialog(taskId) {
  const saved = savedTerminal();
  if (saved) {
    runAttach(taskId, saved);
    return;
  }
  state.attachTaskId = taskId;
  state.attachCli = "";
  updateAttachFallback();
  els.attachDialog.showModal();
}

function openRemoveDialog(task) {
  state.removeTaskId = task.id;
  els.removeSummary.textContent = `Remove #${task.index || task.id} ${task.title}?`;
  els.removeForce.checked = false;
  els.removeDialog.showModal();
}

function handleBoardClick(event) {
  if (!state.selectedId) return;
  const target = event.target;
  if (target.closest(".task-card")) return;
  if (target.closest("dialog")) return;
  state.selectedId = null;
  renderBoard();
}

async function selectTask(taskId) {
  state.selectedId = state.selectedId === taskId ? null : taskId;
  renderBoard();
  if (state.selectedId && !state.details[taskId]) {
    try {
      state.details[taskId] = await request("/tasks/" + encodeURIComponent(taskId));
      renderBoard();
    } catch (err) {
      showToast(err.message);
    }
  }
}

async function addTask() {
  const source = els.addSource.value.trim();
  if (!source) return;
  try {
    await request("/tasks", {
      method: "POST",
      body: JSON.stringify({ source }),
    });
    els.addDialog.close();
    showToast("Task added");
    await refreshTasks();
  } catch (err) {
    showToast(err.message);
  }
}

async function startTask() {
  if (!state.workflow) return;
  try {
    await request("/tasks/" + encodeURIComponent(state.startTaskId) + "/start", {
      method: "POST",
      body: JSON.stringify({
        agent: els.startAgent.value.trim(),
        command: els.startCommand.value.trim(),
        base_branch: els.startBase.value.trim(),
      }),
    });
    if (state.startTarget && state.startTarget !== state.workflow.start) {
      await request("/tasks/" + encodeURIComponent(state.startTaskId) + "/move", {
        method: "POST",
        body: JSON.stringify({ to: state.startTarget === "__done__" ? "done" : state.startTarget }),
      });
    }
    els.startDialog.close();
    showToast("Task started");
    await refreshTasks();
    state.selectedId = state.startTaskId;
    state.details[state.startTaskId] = await request("/tasks/" + encodeURIComponent(state.startTaskId));
    renderBoard();
  } catch (err) {
    showToast(err.message);
  }
}

async function moveTask(taskId, targetColumn) {
  const to = targetColumn === "__done__" ? "done" : targetColumn;
  try {
    await request("/tasks/" + encodeURIComponent(taskId) + "/move", {
      method: "POST",
      body: JSON.stringify({ to }),
    });
    showToast("Moved to " + labelFor(to));
    await refreshTasks();
    if (state.selectedId === taskId) {
      state.details[taskId] = await request("/tasks/" + encodeURIComponent(taskId));
      renderBoard();
    }
  } catch (err) {
    showToast(err.message);
  }
}

async function runAttach(taskId, terminal, fromDialog = false) {
  localStorage.setItem(TERMINAL_KEY, terminal);
  els.attachTerminal.value = terminal;
  try {
    const resp = await request("/tasks/" + encodeURIComponent(taskId) + "/attach", {
      method: "POST",
      body: JSON.stringify({ terminal }),
    });
    if (resp.opened) {
      if (fromDialog) els.attachDialog.close();
      showToast("Attached");
      return;
    }
    state.attachTaskId = taskId;
    state.attachCli = resp.cli || "";
    updateAttachFallback();
    if (!fromDialog && !els.attachDialog.open) els.attachDialog.showModal();
    showToast(resp.error || "Could not open session");
  } catch (err) {
    showToast(err.message);
  }
}

function attachTask() {
  return runAttach(state.attachTaskId, els.attachTerminal.value, true);
}

async function removeTask() {
  try {
    await request("/tasks/" + encodeURIComponent(state.removeTaskId), {
      method: "DELETE",
      body: JSON.stringify({ force: els.removeForce.checked }),
    });
    els.removeDialog.close();
    state.selectedId = null;
    delete state.details[state.removeTaskId];
    showToast("Task removed");
    await refreshTasks();
  } catch (err) {
    showToast(err.message);
  }
}

async function refreshTasks() {
  state.tasks = await request("/tasks");
  const ids = new Set(state.tasks.map((task) => task.id));
  for (const id of Object.keys(state.details)) {
    if (!ids.has(id)) delete state.details[id];
  }
  if (state.selectedId && !ids.has(state.selectedId)) state.selectedId = null;
  renderBoard();
}

async function refreshAll() {
  if (state.polling) return;
  state.polling = true;
  try {
    await refreshTasks();
    if (state.selectedId) {
      state.details[state.selectedId] = await request("/tasks/" + encodeURIComponent(state.selectedId));
      renderBoard();
    }
  } catch (err) {
    showToast(err.message);
  } finally {
    state.polling = false;
  }
}

function handleDragStart(event, task) {
  event.dataTransfer?.setData("text/plain", task.id);
  if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
  requestAnimationFrame(() => {
    state.dragTaskId = task.id;
    renderBoard();
  });
}

function handleDragOver(event, columnId) {
  const task = dragTask();
  if (!task) return;
  event.preventDefault();
  if (event.dataTransfer) event.dataTransfer.dropEffect = canDrop(task, columnId) ? "move" : "none";
}

function handleDrop(event, columnId) {
  event.preventDefault();
  const taskId = event.dataTransfer?.getData("text/plain") || state.dragTaskId;
  state.dragTaskId = null;
  const task = state.tasks.find((item) => item.id === taskId);
  if (!task || !canDrop(task, columnId)) {
    renderBoard();
    return;
  }
  if (columnForTask(task) === "__add__") {
    renderBoard();
    openStartDialog(task, columnId);
    return;
  }
  renderBoard();
  moveTask(task.id, columnId);
}

function handleShortcut(event) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "n") {
    event.preventDefault();
    openAddDialog();
  }
  if (event.key === "Escape" && state.selectedId) {
    state.selectedId = null;
    renderBoard();
  }
}

function taskCard(task) {
  const selected = state.selectedId === task.id;
  const done = isDone(task);
  const classes = ["task-card", selected ? "expanded" : "compact"];
  if (done) classes.push("done");
  if (state.dragTaskId === task.id) classes.push("dragging");

  const meta = create("div", { class: "task-meta" });
  if (task.agent) meta.append(create("span", { class: "agent", text: task.agent }));

  const bodyChildren = [
    create("div", { class: "task-header" }, [
      create("div", { class: "task-top" }, [
        create("span", { class: "task-id", text: `#${task.index || ""}` }),
        meta,
      ]),
      create("h3", { text: task.title || task.id }),
    ]),
  ];

  if (selected) {
    bodyChildren.push(...taskDetail(task));
  }

  const card = create("div", {
    class: classes.join(" "),
    draggable: !done,
    dataset: { taskId: task.id },
    role: "button",
    tabIndex: 0,
    onclick: (event) => {
      if (event.target.closest("button")) return;
      selectTask(task.id);
    },
    onkeydown: (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        selectTask(task.id);
      }
    },
    ondragstart: (event) => handleDragStart(event, task),
    ondragend: () => {
      state.dragTaskId = null;
      renderBoard();
    },
  }, [
    create("div", { class: "task-body" }, bodyChildren),
  ]);

  return card;
}

function taskDetail(task) {
  const detail = state.details[task.id];
  const showAttach = isStarted(task) && !isDone(task);
  const showRemove = columnForTask(task) !== "__add__";
  const children = [
    create("div", { class: "task-scroll" }, [
      create("div", { class: "detail-grid" }, [
        create("section", {}, [
          create("h4", { text: "Source" }),
          create("p", { text: detail?.source || "Loading..." }),
        ]),
        create("section", {}, [
          create("h4", { text: "Handoff" }),
          create("p", { text: detail?.handoff?.content || "-" }),
        ]),
      ]),
    ]),
  ];

  if (showAttach || showRemove) {
    const actions = create("div", { class: "card-actions" });
    if (showAttach) {
      actions.append(create("button", {
        class: "primary",
        type: "button",
        text: "Attach",
        onclick: () => openAttachDialog(task.id),
      }));
    }
    if (showRemove) {
      actions.append(create("button", {
        class: "ghost danger-link",
        type: "button",
        text: "Remove",
        onclick: () => openRemoveDialog(task),
      }));
    }
    children.push(actions);
  }

  return children;
}

function lane(column) {
  const currentDragTask = dragTask();
  const classes = ["lane"];
  if (state.dragTaskId !== null && canDrop(currentDragTask, column.id)) classes.push("drag-over");
  if (state.dragTaskId !== null && !canDrop(currentDragTask, column.id)) classes.push("drag-blocked");

  const columnTasks = tasksForColumn(column.id);
  const body = create("div", { class: "lane-body" });

  if (column.id === "__add__") {
    body.append(create("button", {
      class: "add-row",
      type: "button",
      onclick: openAddDialog,
      text: "+ Add task",
    }));
  }

  for (const task of columnTasks) body.append(taskCard(task));

  return create("section", {
    class: classes.join(" "),
    dataset: { column: column.id },
    role: "list",
    "aria-label": column.label,
    ondragover: (event) => handleDragOver(event, column.id),
    ondrop: (event) => handleDrop(event, column.id),
  }, [
    create("div", { class: "lane-header" }, [
      create("span", { text: column.label }),
      create("span", { class: "lane-count", text: columnTasks.length }),
    ]),
    body,
  ]);
}

function renderBoard() {
  if (!els.board) return;
  const currentColumns = columns();
  els.repoPath.textContent = state.project?.path || "";
  els.board.className = "board" + (state.dragTaskId !== null ? " is-dragging" : "");
  els.board.style.setProperty("--column-count", String(currentColumns.length || 6));
  els.board.replaceChildren(...currentColumns.map(lane));
}

function renderToast() {
  if (!els.toastMount) return;
  els.toastMount.replaceChildren();
  if (!state.toastMessage) return;
  els.toastMount.append(create("div", {
    class: "toast",
    role: "status",
    "aria-live": "polite",
    text: state.toastMessage,
  }));
}

function updateAttachFallback() {
  els.attachCli.textContent = state.attachCli;
  els.attachCli.hidden = !state.attachCli;
}

function mountApp() {
  document.title = "gmc task webui";
  const byId = (id) => document.getElementById(id);
  Object.assign(els, {
    repoPath: document.querySelector(".repo-path"),
    board: document.querySelector(".board"),
    toastMount: byId("toast-mount"),
    addDialog: byId("add-dialog"),
    addSource: byId("add-source"),
    startDialog: byId("start-dialog"),
    startTargetLabel: byId("start-target-label"),
    startAgent: byId("start-agent"),
    startBase: byId("start-base"),
    startCommand: byId("start-command"),
    attachDialog: byId("attach-dialog"),
    attachTerminal: byId("attach-terminal"),
    attachCli: byId("attach-cli"),
    removeDialog: byId("remove-dialog"),
    removeSummary: byId("remove-summary"),
    removeForce: byId("remove-force"),
  });
  byId("add-form").addEventListener("submit", (event) => submit(event, addTask));
  byId("start-form").addEventListener("submit", (event) => submit(event, startTask));
  byId("attach-form").addEventListener("submit", (event) => submit(event, attachTask));
  byId("remove-form").addEventListener("submit", (event) => submit(event, removeTask));
  document.querySelectorAll("[data-close]").forEach((button) => {
    button.addEventListener("click", () => button.closest("dialog").close("cancel"));
  });
}

function submit(event, fn) {
  event.preventDefault();
  fn();
}

async function init() {
  mountApp();
  document.addEventListener("keydown", handleShortcut);
  document.addEventListener("mousedown", handleBoardClick);
  const terminal = savedTerminal();
  if (terminal) els.attachTerminal.value = terminal;

  try {
    state.project = await request("/project");
    state.workflow = await request("/workflow");
    if (!terminal && TERMINALS.includes(state.project.suggested_terminal)) {
      els.attachTerminal.value = state.project.suggested_terminal;
    }
    await refreshTasks();
    state.pollTimer = setInterval(refreshAll, POLL_MS);
  } catch (err) {
    showToast(err.message);
  }
}

window.addEventListener("beforeunload", () => {
  if (state.pollTimer) clearInterval(state.pollTimer);
  if (state.toastTimer) clearTimeout(state.toastTimer);
});

init();
