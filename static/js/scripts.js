document.addEventListener('DOMContentLoaded', function() {
    const loginButton = document.getElementById('login-button');
    const logoutButton = document.getElementById('logout-button');
    const profileSection = document.getElementById('profile-section');
    const loginSection = document.getElementById('login-section');

    if (loginButton) {
        loginButton.addEventListener('click', function() {
            window.location.href = '/github/login';
        });
    }

    if (logoutButton) {
        logoutButton.addEventListener('click', function() {
            // Remove the userinfo cookie
            document.cookie = "userinfo=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
            window.location.href = '/';
        });
    }

    function displayUserInfo(userInfo) {
        try {
            const userNameElement = document.getElementById('userName');
            const userAvatarElement = document.getElementById('userAvatar');
            const profileLinkElement = document.getElementById('profile-link');
            const profileInfoElement = document.getElementById('profile-info');

            if (userNameElement) {
                userNameElement.textContent = userInfo.name;
            }

            if (userAvatarElement) {
                userAvatarElement.src = userInfo.avatar_url;
            }

            if (profileLinkElement) {
                profileLinkElement.href = userInfo.html_url;
            }
           
        } catch (error) {
            console.error("Error parsing userinfo cookie:", error);
        }
    }

    function getCookie(name) {
        const value = `; ${document.cookie}`;
        const parts = value.split(`; ${name}=`);
        if (parts.length === 2) return parts.pop().split(';').shift();
    }

    const userInfoCookie = getCookie('userinfo');

    if (userInfoCookie) {
        try {
            const userInfo = JSON.parse(decodeURIComponent(userInfoCookie));
            displayUserInfo(userInfo);
            profileSection.style.display = 'block';
            loginSection.style.display = 'none';
        } catch (error) {
            console.error("Error parsing userinfo cookie:", error);
        }
    }
});