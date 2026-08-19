let currentWork = null;
let currentTask = null;
let timer = null;

function $(id) {
    return document.getElementById(id);
}

function show(id) {
    $(id).classList.remove("hidden");
}

function hide(id) {
    $(id).classList.add("hidden");
}

function formatSize(size) {

    if (!size || size <= 0) {
        return "-";
    }

    const units = [
        "B",
        "KB",
        "MB",
        "GB",
        "TB"
    ];

    let value = size;
    let index = 0;

    while (
        value >= 1024 &&
        index < units.length - 1
    ) {
        value /= 1024;
        index++;
    }

    return value.toFixed(
        index === 0 ? 0 : 2
    ) + " " + units[index];
}

function showError(message) {

    $("error").textContent = message;

    show("error");
}

function clearError() {

    $("error").textContent = "";

    hide("error");
}

function fileIcon(name) {

    const ext = name
        .split(".")
        .pop()
        .toLowerCase();

    if (
        ext === "wav" ||
        ext === "mp3"
    ) {
        return "🎵";
    }

    if (
        ext === "vtt" ||
        ext === "lrc"
    ) {
        return "📝";
    }

    return "🖼️";
}

async function searchWork() {

    const rj = $("rj").value.trim();

    if (!rj) {
        showError("请输入 RJ 号");
        return;
    }

    clearError();

    $("searchBtn").disabled = true;
    $("searchBtn").textContent = "查询中...";

    hide("work");
    hide("progress");

    try {

        const response = await fetch(
            "/api/work",
            {
                method: "POST",

                headers: {
                    "Content-Type":
                        "application/json"
                },

                body: JSON.stringify({
                    rj: rj
                })
            }
        );

        const data = await response.json();

        if (!response.ok) {
            throw new Error(
                data.error ||
                "查询失败"
            );
        }

        currentWork = data;

        renderWork(data);

        show("work");

    } catch (error) {

        showError(
            error.message ||
            "查询失败"
        );

    } finally {

        $("searchBtn").disabled = false;
        $("searchBtn").textContent = "查询";
    }
}

function renderWork(work) {

    $("workId").textContent =
        "RJ" + work.id;

    $("workTitle").textContent =
        work.title || "未知标题";

    $("fileCount").textContent =
        work.files.length +
        " 个文件";

    const total = work.files.reduce(
        (sum, file) =>
            sum + (file.size || 0),
        0
    );

    $("totalSize").textContent =
        formatSize(total);

    const container = $("files");

    container.innerHTML = "";

    work.files.forEach(
        (file, index) => {

            const row =
                document.createElement("div");

            row.className =
                "file-row";

            row.innerHTML = `
                <label class="file-check">
                    <input
                        type="checkbox"
                        data-index="${index}"
                        checked
                    >
                </label>

                <div class="file-icon">
                    ${fileIcon(file.name)}
                </div>

                <div class="file-info">

                    <div class="file-name">
                        ${escapeHTML(file.name)}
                    </div>

                    <div class="file-path">
                        ${escapeHTML(file.path)}
                    </div>

                </div>

                <div class="file-size">
                    ${formatSize(file.size)}
                </div>
            `;

            container.appendChild(row);
        }
    );
}

function escapeHTML(value) {

    return String(value)
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#039;");
}

function selectAll() {

    document
        .querySelectorAll(
            "#files input[type=checkbox]"
        )
        .forEach(
            checkbox => {
                checkbox.checked = true;
            }
        );
}

function unselectAll() {

    document
        .querySelectorAll(
            "#files input[type=checkbox]"
        )
        .forEach(
            checkbox => {
                checkbox.checked = false;
            }
        );
}

async function startDownload() {

    if (!currentWork) {
        return;
    }

    const selected = [];

    document
        .querySelectorAll(
            "#files input[type=checkbox]"
        )
        .forEach(
            checkbox => {

                if (checkbox.checked) {

                    const index =
                        Number(
                            checkbox.dataset.index
                        );

                    selected.push(
                        currentWork.files[index]
                    );
                }
            }
        );

    if (selected.length === 0) {

        showError(
            "至少选择一个文件"
        );

        return;
    }

    clearError();

    $("downloadBtn").disabled = true;
    $("downloadBtn").textContent =
        "启动中...";

    try {

        const response = await fetch(
            "/api/download",
            {
                method: "POST",

                headers: {
                    "Content-Type":
                        "application/json"
                },

                body: JSON.stringify({
                    rj: currentWork.id,
                    files: selected
                })
            }
        );

        const data =
            await response.json();

        if (!response.ok) {

            throw new Error(
                data.error ||
                "启动下载失败"
            );
        }

        currentTask = data.taskId;

        show("progress");

        $("progress").scrollIntoView({
            behavior: "smooth"
        });

        startPolling();

    } catch (error) {

        showError(
            error.message ||
            "启动下载失败"
        );

    } finally {

        $("downloadBtn").disabled = false;

        $("downloadBtn").textContent =
            "开始下载";
    }
}

function startPolling() {

    if (timer) {
        clearInterval(timer);
    }

    pollTask();

    timer = setInterval(
        pollTask,
        1000
    );
}

async function pollTask() {

    if (!currentTask) {
        return;
    }

    try {

        const response = await fetch(
            "/api/task/" +
            encodeURIComponent(
                currentTask
            )
        );

        if (!response.ok) {
            return;
        }

        const task =
            await response.json();

        renderProgress(task);

        if (
            task.status === "completed" ||
            task.status ===
                "completed_with_errors" ||
            task.status === "failed" ||
            task.status === "cancelled"
        ) {

            clearInterval(timer);
            timer = null;
        }

    } catch (error) {

        console.error(error);
    }
}

function renderProgress(task) {

    $("taskStatus").textContent =
        translateStatus(task.status);

    let percent = 0;

    if (task.totalBytes > 0) {

        percent =
            task.bytesDownloaded /
            task.totalBytes *
            100;

        percent =
            Math.min(
                100,
                Math.max(0, percent)
            );
    } else if (task.total > 0) {

        percent =
            task.success /
            task.total *
            100;
    }

    $("progressValue").style.width =
        percent.toFixed(1) + "%";

    $("progressText").textContent =
        percent.toFixed(1) +
        "%  |  " +
        task.success +
        "/" +
        task.total +
        " 完成";

    const container =
        $("taskFiles");

    container.innerHTML = "";

    task.files.forEach(
        file => {

            let filePercent = 0;

            if (file.size > 0) {

                filePercent =
                    file.downloaded /
                    file.size *
                    100;

                filePercent =
                    Math.min(
                        100,
                        filePercent
                    );
            }

            const row =
                document.createElement(
                    "div"
                );

            row.className =
                "task-file";

            row.innerHTML = `

                <div class="task-file-top">

                    <span>
                        ${escapeHTML(file.path)}
                    </span>

                    <span>
                        ${translateStatus(
                            file.status
                        )}
                    </span>

                </div>

                <div class="small-progress">

                    <div
                        class="small-progress-value"
                        style="width:${filePercent}%"
                    ></div>

                </div>

                <div class="task-file-bottom">

                    <span>
                        ${formatSize(
                            file.downloaded
                        )}
                        /
                        ${formatSize(
                            file.size
                        )}
                    </span>

                    <span>
                        ${filePercent.toFixed(1)}%
                    </span>

                </div>
            `;

            container.appendChild(row);
        }
    );
}

function translateStatus(status) {

    const map = {

        waiting: "等待",

        downloading: "下载中",

        completed: "完成",

        skipped: "已存在",

        failed: "失败",

        cancelled: "已取消",

        completed_with_errors:
            "完成，但有失败"

    };

    return map[status] || status;
}

$("rj").addEventListener(
    "keydown",
    event => {

        if (event.key === "Enter") {
            searchWork();
        }
    }
);