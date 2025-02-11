document.getElementById('login-button').addEventListener('click', () => {
    window.location.href = '/github/login';
});

document.getElementById('logout-button').addEventListener('click', () => {
    document.cookie = 'Authorization=; Max-Age=0; path=/';
    document.getElementById('login-section').style.display = 'block';
    document.getElementById('profile-section').style.display = 'none';
});

function fetchProfile() {
    axios.get('/github/profile')
    .then(response => {
        const profileInfo = document.getElementById('profile-info');
        const profileLink = document.getElementById('profile-link');
        profileInfo.textContent = JSON.stringify(response.data, null, 2);
        profileLink.href = response.data.html_url;
        document.getElementById('login-section').style.display = 'none';
        document.getElementById('profile-section').style.display = 'block';
    })
    .catch(error => {
        console.error('Error fetching profile:', error);
    });
}

// Call fetchProfile on page load to display the profile if the user is already logged in
window.onload = fetchProfile;