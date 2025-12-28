/**
 * 登录页面初始化和登录处理
 */
(function() {
    'use strict';

    // 页面加载时检查会话状态
    checkSession();

    // 获取登录表单并添加提交事件监听
    const form = document.getElementById('loginForm');
    if (form) {
        form.addEventListener('submit', handleLogin);
    }

    /**
     * 从cookie中获取CSRF token
     */
    function getCsrfToken() {
        const match = document.cookie.split('; ').find(row => row.startsWith('csrf_token='));
        return match ? decodeURIComponent(match.split('=')[1]) : '';
    }

    /**
     * 检查用户会话状态，如果已登录则重定向到首页
     */
    async function checkSession() {
        try {
            const response = await fetch('/api/session');
            if (response.ok) {
                const data = await response.json();
                if (data.authenticated) {
                    window.location.href = '/';
                }
            }
        } catch (error) {
            console.debug('检查会话出错:', error);
        }
    }

    /**
     * 处理登录表单提交
     */
    async function handleLogin(event) {
        event.preventDefault();

        const username = document.getElementById('username').value.trim();
        const password = document.getElementById('password').value;
        const loginBtn = document.getElementById('loginBtn');
        const btnText = loginBtn.querySelector('.btn-text');
        const btnLoading = loginBtn.querySelector('.btn-loading');
        const errorEl = document.getElementById('errorMessage');

        // 清除之前的错误信息
        errorEl.style.display = 'none';

        // 验证输入字段
        if (!username || !password) {
            showError('请输入用户名和密码');
            return;
        }

        // 显示加载状态
        setLoading(true);

        try {
            const csrfToken = getCsrfToken();
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': csrfToken
                },
                body: JSON.stringify({ username, password })
            });

            const data = await response.json();

            if (response.ok && data.success) {
                // 登录成功，重定向到首页
                window.location.href = '/';
            } else {
                // 显示错误信息
                showError(data.error || '登录失败，请重试');
            }
        } catch (error) {
            console.error('登录请求出错:', error);
            showError('网络错误，请检查连接后重试');
        } finally {
            setLoading(false);
        }
    }

    /**
     * 显示错误信息
     */
    function showError(message) {
        const errorEl = document.getElementById('errorMessage');
        errorEl.textContent = message;
        errorEl.style.display = 'block';
    }

    /**
     * 设置加载状态
     */
    function setLoading(loading) {
        const loginBtn = document.getElementById('loginBtn');
        const btnText = loginBtn.querySelector('.btn-text');
        const btnLoading = loginBtn.querySelector('.btn-loading');

        loginBtn.disabled = loading;
        btnText.style.display = loading ? 'none' : 'inline';
        btnLoading.style.display = loading ? 'flex' : 'none';
    }
})();
