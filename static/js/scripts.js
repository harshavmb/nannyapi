document.getElementById('login-button').addEventListener('click', () => {
    window.location.href = '/github/login';
});

document.getElementById('logout-button').addEventListener('click', () => {
    document.cookie = 'Authorization=; Max-Age=0; path=/';
    document.cookie = 'userinfo=; Max-Age=0; path=/';
    document.getElementById('login-section').style.display = 'block';
    document.getElementById('profile-section').style.display = 'none';
});

function getCookie(name) {
    const value = `; ${document.cookie}`;
    const parts = value.split(`; ${name}=`);
    if (parts.length === 2) {
        const cookieValue = parts.pop().split(';').shift();
        return cookieValue;
    }
    console.log('Cookie not found:', name); // Log if the cookie is not found
    return undefined;
}

function displayUserInfo() {
    const userInfoCookie = getCookie('userinfo');

    if (userInfoCookie) {
        try {
            // URL-decode the cookie value
            const decodedUserInfoCookie = decodeURIComponent(userInfoCookie).replace(/\+/g, ' ');
            const userInfo = JSON.parse(decodedUserInfoCookie);

            document.getElementById('login-section').style.display = 'none';
            document.getElementById('profile-section').style.display = 'block';

            if (userInfo.name) {
                document.getElementById('userName').textContent = userInfo.name;
            }

            if (userInfo.avatar_url) {
                document.getElementById('userAvatar').src = userInfo.avatar_url;
            }

            if (userInfo.html_url) {
                document.getElementById('userProfile').src = userInfo.html_url;
            }
        } catch (error) {
            console.error('Error parsing userinfo cookie:', error);
        }
    } else {
        document.getElementById('login-section').style.display = 'block';
        document.getElementById('profile-section').style.display = 'none';
    }
}

// Call displayUserInfo on page load to display the profile if the user is already logged in
window.onload = displayUserInfo;