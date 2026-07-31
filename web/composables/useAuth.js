import { ref, computed, watch } from 'vue';

export function useAuth() {
  const showAuthModal = ref(false);
  const showSetupModal = ref(false);
  const showProfileModal = ref(false);
  const profileForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '', newUsername: '' });
  const profileError = ref('');
  const profileSuccess = ref('');
  const authMode = ref('login');
  const authForm = ref({ username: '', password: '', confirmPassword: '' });
  const authError = ref('');
  const currentUser = ref(null);
  const token = ref(localStorage.getItem('token') || null);

  const showWebAuthModal = ref(false);
  const webAuthPassword = ref('');
  const webAuthError = ref('');
  const webPasswordRequired = ref(false);

  const showUserMenu = ref(false);

  const fetchWithAuth = (url, options = {}) => {
    const headers = { 'Content-Type': 'application/json', ...options.headers };
    if (token.value) headers.Authorization = `Bearer ${token.value}`;
    return fetch(url, { ...options, headers });
  };

  const handleLogout = () => {
    token.value = null;
    currentUser.value = null;
    localStorage.removeItem('token');
  };

  const openAuthModal = (mode = 'login') => {
    authMode.value = mode;
    authForm.value = { username: '', password: '', confirmPassword: '' };
    authError.value = '';
    showAuthModal.value = true;
  };

  const closeAuthModal = () => {
    showAuthModal.value = false;
    authError.value = '';
  };

  const handleAuth = async () => {
    try {
      const res = await fetch('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: authForm.value.username, password: authForm.value.password })
      });
      const data = await res.json();
      if (res.ok) {
        token.value = data.token;
        currentUser.value = data.user;
        localStorage.setItem('token', data.token);
        closeAuthModal();
      } else {
        authError.value = data.message || 'Неверное имя пользователя или пароль';
      }
    } catch (e) {
      console.error('Ошибка входа:', e);
      authError.value = 'Произошла ошибка при попытке авторизации';
    }
  };

  const handleSetup = async () => {
    if (authForm.value.password.length < 3) {
      authError.value = 'Пароль должен содержать минимум 3 символа';
      return;
    }
    try {
      const res = await fetch('/api/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: authForm.value.username, password: authForm.value.password })
      });
      const data = await res.json();
      if (res.ok) {
        token.value = data.token;
        currentUser.value = data.user;
        localStorage.setItem('token', data.token);
        showSetupModal.value = false;
        authError.value = '';
      } else {
        authError.value = data.message || 'Ошибка настройки';
      }
    } catch (e) {
      authError.value = 'Произошла ошибка';
    }
  };

  const checkSetupRequired = async () => {
    try {
      const res = await fetch('/api/setup-status');
      if (res.ok) {
        const data = await res.json();
        if (data.setup_required) showSetupModal.value = true;
      }
    } catch (e) {}
  };

  const openProfileModal = () => {
    profileForm.value = {
      oldPassword: '',
      newPassword: '',
      confirmPassword: '',
      newUsername: currentUser.value?.username || ''
    };
    profileError.value = '';
    profileSuccess.value = '';
    showProfileModal.value = true;
  };

  const handleUpdateProfile = async () => {
    profileError.value = '';
    profileSuccess.value = '';
    if (profileForm.value.newPassword && profileForm.value.newPassword !== profileForm.value.confirmPassword) {
      profileError.value = 'Пароли не совпадают';
      return;
    }
    if (profileForm.value.newPassword && profileForm.value.newPassword.length < 3) {
      profileError.value = 'Пароль должен содержать минимум 3 символа';
      return;
    }
    try {
      const body = {};
      if (profileForm.value.newPassword) {
        body.old_password = profileForm.value.oldPassword;
        body.new_password = profileForm.value.newPassword;
      }
      if (profileForm.value.newUsername && profileForm.value.newUsername !== currentUser.value?.username) {
        body.new_username = profileForm.value.newUsername;
      }
      if (Object.keys(body).length === 0) {
        profileError.value = 'Укажите новый пароль или имя';
        return;
      }
      if (body.new_password && !body.old_password) {
        profileError.value = 'Для смены пароля укажите старый пароль';
        return;
      }
      const res = await fetchWithAuth('/api/update-profile', {
        method: 'POST',
        body: JSON.stringify(body)
      });
      const data = await res.json();
      if (res.ok) {
        profileSuccess.value = 'Профиль обновлён';
        if (body.new_username) currentUser.value = { ...currentUser.value, username: body.new_username };
      } else {
        profileError.value = data.message || 'Ошибка обновления';
      }
    } catch (e) {
      profileError.value = 'Ошибка связи с сервером';
    }
  };

  const closeUserMenu = () => {
    showUserMenu.value = false;
  };

  const checkWebAuthStatus = async () => {
    try {
      const res = await fetch('/api/web-auth-status');
      if (res.ok) {
        const data = await res.json();
        webPasswordRequired.value = data.password_required;
        if (data.password_required) showWebAuthModal.value = true;
      }
    } catch (e) {
      console.error(e);
    }
  };

  const handleWebAuth = async () => {
    try {
      const res = await fetch('/api/web-auth', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: webAuthPassword.value })
      });
      if (res.ok) {
        document.cookie = 'web_auth_session=authenticated; path=/; max-age=2592000';
        showWebAuthModal.value = false;
        webAuthError.value = '';
        webAuthPassword.value = '';
      } else {
        webAuthError.value = 'Неверный пароль';
      }
    } catch (e) {
      webAuthError.value = 'Произошла ошибка';
    }
  };

  const userInitial = computed(() => {
    const u = currentUser.value?.username;
    if (!u || !String(u).trim()) return '?';
    return String(u).trim()[0].toLocaleUpperCase('ru-RU');
  });

  watch(currentUser, (u) => {
    if (!u) showUserMenu.value = false;
  });

  return {
    showAuthModal,
    showSetupModal,
    showProfileModal,
    profileForm,
    profileError,
    profileSuccess,
    authMode,
    authForm,
    authError,
    currentUser,
    token,
    fetchWithAuth,
    handleLogout,
    openAuthModal,
    closeAuthModal,
    handleAuth,
    handleSetup,
    checkSetupRequired,
    openProfileModal,
    handleUpdateProfile,
    showWebAuthModal,
    webAuthPassword,
    webAuthError,
    webPasswordRequired,
    checkWebAuthStatus,
    handleWebAuth,
    showUserMenu,
    closeUserMenu,
    userInitial
  };
}
