document.addEventListener('DOMContentLoaded', function() {
    // Add event listeners to delete buttons
    const deleteButtons = document.querySelectorAll('.delete-button');
    deleteButtons.forEach(button => {
        button.addEventListener('click', deleteAuthToken);
    });

    // Function to delete an auth token
    async function deleteAuthToken(event) {
        const tokenId = event.target.dataset.tokenId;
        if (!confirm('Are you sure you want to delete this auth token?')) {
            return;
        }

        try {
            await axios.delete(`/auth-token/${tokenId}`);
            // Refresh the auth tokens table
            window.location.reload();
        } catch (error) {
            console.error('Error deleting auth token:', error);
            alert('Failed to delete auth token.');
        }
    }
});