/**
 * Token Dashboard - 前端控制器
 * 基于模块化设计，遵循单一职责原则
 */

let dashboard; // 全局变量，供HTML调用

class TokenDashboard {
    constructor() {
        this.autoRefreshInterval = null;
        this.isAutoRefreshEnabled = false;
        this.apiBaseUrl = '/api';
        this.pendingDeleteIndex = null;

        this.init();
    }

    /**
     * 初始化Dashboard
     */
    init() {
        this.bindEvents();
        this.checkSession(); // 检查会话状态
        this.refreshTokens();
    }

    /**
     * 从 cookie 获取 CSRF token
     */
    getCsrfToken() {
        const match = document.cookie.split('; ').find(row => row.startsWith('csrf_token='));
        return match ? decodeURIComponent(match.split('=')[1]) : '';
    }

    /**
     * 检查会话状态，显示/隐藏登出按钮
     */
    async checkSession() {
        try {
            const response = await fetch(`${this.apiBaseUrl}/session`);
            if (response.ok) {
                const data = await response.json();
                const logoutBtn = document.getElementById('logoutBtn');
                if (logoutBtn) {
                    logoutBtn.style.display = data.authenticated ? 'inline-block' : 'none';
                }
            }
        } catch (error) {
            // 会话检查失败，可能未启用登录系统
            console.debug('会话检查失败:', error);
        }
    }

    /**
     * 登出
     */
    async logout() {
        try {
            const response = await fetch(`${this.apiBaseUrl}/logout`, {
                method: 'POST',
                headers: {
                    'X-CSRF-Token': this.getCsrfToken()
                }
            });
            if (response.ok) {
                window.location.href = '/static/login.html';
            } else {
                this.showToast('登出失败', 'error');
            }
        } catch (error) {
            console.error('登出请求失败:', error);
            this.showToast('网络错误', 'error');
        }
    }

    /**
     * 绑定事件处理器 (DRY原则)
     */
    bindEvents() {
        // 手动刷新按钮
        const refreshBtn = document.querySelector('.refresh-btn');
        if (refreshBtn) {
            refreshBtn.addEventListener('click', () => this.refreshTokens());
        }

        // 自动刷新开关
        const switchEl = document.querySelector('.switch');
        if (switchEl) {
            switchEl.addEventListener('click', () => this.toggleAutoRefresh());
        }

        // 点击模态框外部关闭
        window.addEventListener('click', (e) => {
            if (e.target.classList.contains('modal')) {
                this.hideAddTokenModal();
                this.hideDeleteConfirmModal();
            }
        });
    }

    /**
     * 获取Token数据 - 简单直接 (KISS原则)
     */
    async refreshTokens() {
        const tbody = document.getElementById('tokenTableBody');
        this.showLoading(tbody, '正在刷新Token数据...');

        try {
            const response = await fetch(`${this.apiBaseUrl}/tokens`);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            const data = await response.json();
            this.updateTokenTable(data);
            this.updateStatusBar(data);
            this.updateLastUpdateTime();

        } catch (error) {
            console.error('刷新Token数据失败:', error);
            this.showError(tbody, `加载失败: ${error.message}`);
        }
    }

    /**
     * 更新Token表格 (OCP原则 - 易于扩展新字段)
     */
    updateTokenTable(data) {
        const tbody = document.getElementById('tokenTableBody');

        if (!data.tokens || data.tokens.length === 0) {
            this.showEmpty(tbody);
            return;
        }

        const rows = data.tokens.map((token, index) => this.createTokenRow(token, index)).join('');
        tbody.innerHTML = rows;
    }

    /**
     * 创建单个Token行 (SRP原则)
     */
    createTokenRow(token, index) {
        const statusClass = this.getStatusClass(token);
        const statusText = this.getStatusText(token);
        const errorMsg = this.getErrorMessage(token);

        // 如果有错误，显示带tooltip的状态徽章
        const statusBadge = errorMsg
            ? `<span class="status-badge ${statusClass}" title="${errorMsg}">${statusText}</span>
               <div class="error-hint">${errorMsg}</div>`
            : `<span class="status-badge ${statusClass}">${statusText}</span>`;

        // 判断是否需要显示刷新按钮（失效状态：错误、过期、耗尽、未初始化）
        const needsRefresh = token.error ||
            token.status === 'error' ||
            token.status === 'pending' ||
            new Date(token.expires_at) < new Date() ||
            (token.remaining_usage || 0) === 0;

        const refreshButton = needsRefresh
            ? `<button class="btn-refresh-small" onclick="dashboard.refreshSingleToken(${index})" title="刷新此Token">刷新</button>`
            : '';

        return `
            <tr class="${token.error ? 'row-error' : ''}">
                <td>${token.user_email || 'unknown'}</td>
                <td><span class="token-preview">${token.token_preview || 'N/A'}</span></td>
                <td>${token.auth_type || 'Social'}</td>
                <td>${token.remaining_usage || 0}</td>
                <td>${this.formatDateTime(token.expires_at)}</td>
                <td>${this.formatDateTime(token.last_used)}</td>
                <td class="status-cell">${statusBadge}</td>
                <td>
                    ${refreshButton}
                    <button class="btn-delete-small" onclick="dashboard.showDeleteConfirmModal(${index})">删除</button>
                </td>
            </tr>
        `;
    }

    /**
     * 显示空状态
     */
    showEmpty(container) {
        container.innerHTML = `
            <tr>
                <td colspan="8" class="empty-state">
                    <div class="empty-icon">📭</div>
                    <p>暂无Token数据</p>
                    <p class="empty-hint">点击上方"添加账号"按钮添加第一个账号</p>
                </td>
            </tr>
        `;
    }

    /**
     * 更新状态栏 (SRP原则)
     */
    updateStatusBar(data) {
        this.updateElement('totalTokens', data.total_tokens || 0);
        this.updateElement('activeTokens', data.active_tokens || 0);
    }

    /**
     * 更新最后更新时间
     */
    updateLastUpdateTime() {
        const now = new Date();
        const timeStr = now.toLocaleTimeString('zh-CN', { hour12: false });
        this.updateElement('lastUpdate', timeStr);
    }

    /**
     * 切换自动刷新 (ISP原则 - 接口隔离)
     */
    toggleAutoRefresh() {
        const switchEl = document.querySelector('.switch');

        if (this.isAutoRefreshEnabled) {
            this.stopAutoRefresh();
            switchEl.classList.remove('active');
        } else {
            this.startAutoRefresh();
            switchEl.classList.add('active');
        }
    }

    /**
     * 启动自动刷新
     */
    startAutoRefresh() {
        this.autoRefreshInterval = setInterval(() => this.refreshTokens(), 30000);
        this.isAutoRefreshEnabled = true;
    }

    /**
     * 停止自动刷新
     */
    stopAutoRefresh() {
        if (this.autoRefreshInterval) {
            clearInterval(this.autoRefreshInterval);
            this.autoRefreshInterval = null;
        }
        this.isAutoRefreshEnabled = false;
    }

    // ==================== 添加账号功能 ====================

    /**
     * 显示添加账号模态框
     */
    showAddTokenModal() {
        document.getElementById('addTokenModal').style.display = 'flex';
        this.resetAddTokenForm();
    }

    /**
     * 隐藏添加账号模态框
     */
    hideAddTokenModal() {
        document.getElementById('addTokenModal').style.display = 'none';
        this.resetAddTokenForm();
    }

    /**
     * 重置添加表单
     */
    resetAddTokenForm() {
        document.getElementById('authType').value = 'Social';
        document.getElementById('refreshToken').value = '';
        document.getElementById('clientId').value = '';
        document.getElementById('clientSecret').value = '';
        document.getElementById('idcFields').style.display = 'none';
        document.getElementById('addTokenError').style.display = 'none';
        // 重置 JSON 输入
        document.getElementById('jsonInput').value = '';
        // 重置 Tab 到手动输入
        this.switchTab('manual');
    }

    /**
     * 切换 Tab
     */
    switchTab(tabName) {
        // 更新 Tab 按钮状态
        document.querySelectorAll('.tab-btn').forEach((btn, index) => {
            btn.classList.toggle('active',
                (tabName === 'manual' && index === 0) ||
                (tabName === 'json' && index === 1)
            );
        });

        // 更新面板显示
        document.getElementById('manualPanel').classList.toggle('active', tabName === 'manual');
        document.getElementById('jsonPanel').classList.toggle('active', tabName === 'json');

        // 清除错误信息
        document.getElementById('addTokenError').style.display = 'none';
    }

    /**
     * 解析 JSON 输入并填充表单
     */
    parseJsonInput() {
        const jsonInput = document.getElementById('jsonInput').value.trim();

        if (!jsonInput) {
            this.showFormError('请输入 JSON 配置');
            return;
        }

        try {
            const config = JSON.parse(jsonInput);

            // 验证必要字段
            if (!config.refreshToken) {
                this.showFormError('JSON 中缺少 refreshToken 字段');
                return;
            }

            // 填充表单
            const authType = config.auth || 'Social';
            document.getElementById('authType').value = authType;
            document.getElementById('refreshToken').value = config.refreshToken || '';
            document.getElementById('clientId').value = config.clientId || '';
            document.getElementById('clientSecret').value = config.clientSecret || '';

            // 显示/隐藏 IdC 字段
            document.getElementById('idcFields').style.display =
                authType === 'IdC' ? 'block' : 'none';

            // 切换到手动输入 Tab 显示填充结果
            this.switchTab('manual');

            // 显示成功提示
            this.showToast('JSON 解析成功，已填充表单');

        } catch (e) {
            this.showFormError('JSON 格式无效: ' + e.message);
        }
    }

    /**
     * 切换IdC字段显示
     */
    toggleIdcFields() {
        const authType = document.getElementById('authType').value;
        const idcFields = document.getElementById('idcFields');
        idcFields.style.display = authType === 'IdC' ? 'block' : 'none';
    }

    /**
     * 添加Token
     */
    async addToken() {
        const authType = document.getElementById('authType').value;
        const refreshToken = document.getElementById('refreshToken').value.trim();
        const clientId = document.getElementById('clientId').value.trim();
        const clientSecret = document.getElementById('clientSecret').value.trim();
        const errorEl = document.getElementById('addTokenError');

        // 验证
        if (!refreshToken) {
            this.showFormError('请输入 Refresh Token');
            return;
        }

        if (authType === 'IdC' && (!clientId || !clientSecret)) {
            this.showFormError('IdC认证需要提供 Client ID 和 Client Secret');
            return;
        }

        // 构建请求数据
        const data = {
            auth: authType,
            refreshToken: refreshToken
        };

        if (authType === 'IdC') {
            data.clientId = clientId;
            data.clientSecret = clientSecret;
        }

        try {
            const response = await fetch(`${this.apiBaseUrl}/tokens`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': this.getCsrfToken()
                },
                body: JSON.stringify(data)
            });

            const result = await response.json();

            if (result.success) {
                this.hideAddTokenModal();
                this.refreshTokens();
                this.showToast('账号添加成功');
            } else {
                this.showFormError(result.error || '添加失败');
            }
        } catch (error) {
            console.error('添加Token失败:', error);
            this.showFormError('网络错误: ' + error.message);
        }
    }

    /**
     * 显示表单错误
     */
    showFormError(message) {
        const errorEl = document.getElementById('addTokenError');
        errorEl.textContent = message;
        errorEl.style.display = 'block';
    }

    // ==================== 删除账号功能 ====================

    /**
     * 刷新单个Token
     */
    async refreshSingleToken(index) {
        try {
            const response = await fetch(`${this.apiBaseUrl}/tokens/${index}/refresh`, {
                method: 'POST',
                headers: {
                    'X-CSRF-Token': this.getCsrfToken()
                }
            });

            const result = await response.json();

            if (result.success) {
                this.showToast('刷新已触发，请稍后刷新页面查看状态');
                // 3秒后自动刷新列表
                setTimeout(() => this.refreshTokens(), 3000);
            } else {
                this.showToast(result.message || '刷新失败', 'error');
            }
        } catch (error) {
            console.error('刷新Token失败:', error);
            this.showToast('网络错误: ' + error.message, 'error');
        }
    }

    /**
     * 显示删除确认模态框
     */
    showDeleteConfirmModal(index) {
        this.pendingDeleteIndex = index;
        document.getElementById('deleteConfirmModal').style.display = 'flex';
    }

    /**
     * 隐藏删除确认模态框
     */
    hideDeleteConfirmModal() {
        this.pendingDeleteIndex = null;
        document.getElementById('deleteConfirmModal').style.display = 'none';
    }

    /**
     * 确认删除Token
     */
    async confirmDeleteToken() {
        if (this.pendingDeleteIndex === null) return;

        try {
            const response = await fetch(`${this.apiBaseUrl}/tokens/${this.pendingDeleteIndex}`, {
                method: 'DELETE',
                headers: {
                    'X-CSRF-Token': this.getCsrfToken()
                }
            });

            const result = await response.json();

            if (result.success) {
                this.hideDeleteConfirmModal();
                this.refreshTokens();
                this.showToast('账号删除成功');
            } else {
                this.showToast(result.error || '删除失败', 'error');
            }
        } catch (error) {
            console.error('删除Token失败:', error);
            this.showToast('网络错误: ' + error.message, 'error');
        }
    }

    // ==================== 工具方法 ====================

    /**
     * 显示提示消息
     */
    showToast(message, type = 'success') {
        // 移除现有的toast
        const existingToast = document.querySelector('.toast');
        if (existingToast) {
            existingToast.remove();
        }

        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        toast.textContent = message;
        document.body.appendChild(toast);

        // 显示动画
        setTimeout(() => toast.classList.add('show'), 10);

        // 自动隐藏
        setTimeout(() => {
            toast.classList.remove('show');
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    }

    /**
     * 工具方法 - 状态判断 (KISS原则)
     */
    getStatusClass(token) {
        // 优先检查错误状态
        if (token.status === 'error' || token.error) {
            return 'status-error';
        }
        if (token.status === 'disabled') {
            return 'status-disabled';
        }
        if (token.status === 'pending') {
            return 'status-pending';
        }
        if (new Date(token.expires_at) < new Date()) {
            return 'status-expired';
        }
        const remaining = token.remaining_usage || 0;
        if (remaining === 0) return 'status-exhausted';
        if (remaining <= 5) return 'status-low';
        return 'status-active';
    }

    getStatusText(token) {
        // 优先检查错误状态
        if (token.status === 'error' || token.error) {
            return '凭证无效';
        }
        if (token.status === 'disabled') {
            return '已禁用';
        }
        if (token.status === 'pending') {
            return '未初始化';
        }
        if (new Date(token.expires_at) < new Date()) {
            return '已过期';
        }
        const remaining = token.remaining_usage || 0;
        if (remaining === 0) return '已耗尽';
        if (remaining <= 5) return '即将耗尽';
        return '正常';
    }

    /**
     * 获取错误提示信息
     */
    getErrorMessage(token) {
        if (!token.error) return '';
        // 简化错误信息显示
        if (token.error.includes('401') || token.error.includes('Bad credentials')) {
            return 'Refresh Token 无效或已过期，请重新获取';
        }
        if (token.error.includes('403')) {
            return '账号权限不足';
        }
        if (token.error.includes('429')) {
            return '请求过于频繁，请稍后重试';
        }
        return token.error;
    }

    /**
     * 工具方法 - 日期格式化 (DRY原则)
     */
    formatDateTime(dateStr) {
        if (!dateStr) return '-';

        try {
            const date = new Date(dateStr);
            if (isNaN(date.getTime())) return '-';

            return date.toLocaleString('zh-CN', {
                year: 'numeric',
                month: '2-digit',
                day: '2-digit',
                hour: '2-digit',
                minute: '2-digit',
                hour12: false
            });
        } catch (e) {
            return '-';
        }
    }

    /**
     * UI工具方法 (KISS原则)
     */
    updateElement(id, content) {
        const element = document.getElementById(id);
        if (element) element.textContent = content;
    }

    showLoading(container, message) {
        container.innerHTML = `
            <tr>
                <td colspan="8" class="loading">
                    <div class="spinner"></div>
                    ${message}
                </td>
            </tr>
        `;
    }

    showError(container, message) {
        container.innerHTML = `
            <tr>
                <td colspan="8" class="error">
                    ${message}
                </td>
            </tr>
        `;
    }
}

// DOM加载完成后初始化 (依赖注入原则)
document.addEventListener('DOMContentLoaded', () => {
    dashboard = new TokenDashboard();
});