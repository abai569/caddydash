// js/settings.js - 设置页面的逻辑

import { initializePage } from './common.js';
import { api } from './api.js';
import { notification } from './notifications.js';
import { t, setLanguage, getCurrentLanguage } from './locale.js';
import { createCustomSelect } from './ui.js';

function pageInit() {
    const DOMElements = {
        resetForm: document.getElementById('reset-password-form'),
        logoutBtn: document.getElementById('logout-btn'),
        backupDownloadBtn: document.getElementById('backup-download-btn'),
        backupRestoreBtn: document.getElementById('backup-restore-btn'),
        backupFileInput: document.getElementById('backup-file-input'),
    };
    const resetButton = DOMElements.resetForm ? DOMElements.resetForm.querySelector('button[type="submit"]') : null;

    async function handleResetPassword(e) {
        e.preventDefault();
        if (!DOMElements.resetForm) return;
        const newPassword = DOMElements.resetForm.new_password.value;
        const confirmPassword = DOMElements.resetForm.confirm_new_password.value;
        const currentPassword = DOMElements.resetForm.old_password.value;
        const username = DOMElements.resetForm.username.value;

        if (!username || !currentPassword || !newPassword || !confirmPassword) {
            notification.toast(t('toasts.error_all_fields_required'), 'error');
            return;
        }
        if (newPassword !== confirmPassword) {
            notification.toast(t('toasts.init_error_mismatch'), 'error');
            return;
        }
        if (newPassword.length < 8) {
            notification.toast(t('toasts.init_error_short'), 'error');
            return;
        }

        if (resetButton) {
            resetButton.disabled = true;
            resetButton.querySelector('span').textContent = t('pages.settings.resetting_password_btn');
        }

        try {
            const response = await fetch('/v0/api/auth/resetpwd', {
                method: 'POST',
                body: new URLSearchParams(new FormData(DOMElements.resetForm)),
            });
            const result = await response.json();
            if (response.ok) {
                notification.toast(t('toasts.pwd_reset_success'), 'success');
                setTimeout(() => { window.location.href = '/v0/api/auth/logout'; }, 1500);
            } else {
                throw new Error(result.error || t('toasts.reset_password_error'));
            }
        } catch (error) {
            notification.toast(`${t('common.error_prefix')}: ${error.message}`, 'error');
            if (resetButton) {
                resetButton.disabled = false;
                resetButton.querySelector('span').textContent = t('pages.settings.reset_password_btn');
            }
        }
    }

    async function handleLogout() {
        if (await notification.confirm(t('dialogs.logout_msg'))) {
            notification.toast(t('toasts.logout_processing'), 'info');
            setTimeout(() => { window.location.href = '/v0/api/auth/logout'; }, 500);
        }
    }

    async function handleBackupDownload() {
        const btn = DOMElements.backupDownloadBtn;
        if (!btn) return;
        const origText = btn.querySelector('span').textContent;
        btn.disabled = true;
        btn.querySelector('span').textContent = t('pages.settings.backuping_btn');

        try {
            const response = await fetch('/v0/api/backup/download');
            if (response.redirected) {
                window.location.href = response.url;
                return;
            }
            if (!response.ok) {
                const errorData = await response.json().catch(() => ({ error: `HTTP error! status: ${response.status}` }));
                throw new Error(errorData.error);
            }
            const blob = await response.blob();
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            const fileName = response.headers.get('content-disposition')?.match(/filename="(.+)"/)?.[1] || 'caddydash-backup.zip';
            a.download = fileName;
            document.body.appendChild(a);
            a.click();
            a.remove();
            window.URL.revokeObjectURL(url);
            notification.toast(t('toasts.backup_started'), 'success');
        } catch (error) {
            notification.toast(`${t('common.error_prefix')}: ${error.message}`, 'error');
        } finally {
            btn.disabled = false;
            btn.querySelector('span').textContent = origText;
        }
    }

    async function triggerBackupRestore() {
        if (!DOMElements.backupFileInput) return;
        DOMElements.backupFileInput.value = '';
        DOMElements.backupFileInput.click();
    }

    async function handleRestoreBackup() {
        if (!DOMElements.backupFileInput) return;
        const file = DOMElements.backupFileInput.files[0];
        if (!file) return;
        if (!file.name.endsWith('.zip')) {
            notification.toast(t('toasts.restore_invalid_file'), 'error');
            return;
        }

        const confirmed = await notification.confirm(t('toasts.restore_confirm'), t('dialogs.restore_title'), {
            confirmText: t('dialogs.confirm_btn'),
            cancelText: t('dialogs.cancel_btn'),
        });
        if (!confirmed) return;

        const btn = DOMElements.backupRestoreBtn;
        if (!btn) return;
        const origText = btn.querySelector('span').textContent;
        btn.disabled = true;
        btn.querySelector('span').textContent = t('pages.settings.restoring_btn');

        const formData = new FormData();
        formData.append('backup', file);

        try {
            const response = await fetch('/v0/api/backup/restore', {
                method: 'POST',
                body: formData,
            });
            const result = await response.json();
            if (response.ok) {
                notification.toast(result.message || t('toasts.restore_success'), 'success');
                setTimeout(() => { window.location.reload(); }, 1500);
            } else {
                throw new Error(result.error);
            }
        } catch (error) {
            notification.toast(`${t('common.error_prefix')}: ${error.message}`, 'error');
        } finally {
            btn.disabled = false;
            btn.querySelector('span').textContent = origText;
            DOMElements.backupFileInput.value = '';
        }
    }

    // 语言选择器
    const langOptions = { 'en': 'English', 'zh-CN': '简体中文' };
    const langSelectOptions = Object.keys(langOptions).map(key => ({ name: langOptions[key], value: key }));

    createCustomSelect('select-language', langSelectOptions, (selectedValue) => {
        setLanguage(selectedValue);
    });

    const langSelect = document.getElementById('select-language');
    if (langSelect) {
        const currentLangName = langOptions[getCurrentLanguage()];
        const selectedDiv = langSelect.querySelector('.select-selected');
        if (selectedDiv) selectedDiv.textContent = currentLangName;
    }

    // 绑定事件
    if (DOMElements.resetForm) DOMElements.resetForm.addEventListener('submit', handleResetPassword);
    if (DOMElements.logoutBtn) DOMElements.logoutBtn.addEventListener('click', handleLogout);
    if (DOMElements.backupDownloadBtn) DOMElements.backupDownloadBtn.addEventListener('click', handleBackupDownload);
    if (DOMElements.backupRestoreBtn) DOMElements.backupRestoreBtn.addEventListener('click', triggerBackupRestore);
    if (DOMElements.backupFileInput) DOMElements.backupFileInput.addEventListener('change', handleRestoreBackup);
}

initializePage({ pageId: 'settings', pageInit: pageInit });