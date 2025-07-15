document.addEventListener('DOMContentLoaded', function() {
    // Extract share token from URL 
    const pathParts = window.location.pathname.split('/');
    const shareToken = pathParts[pathParts.length - 1];
    
    if (!shareToken) {
        showError("Invalid share link");
        return;
    }
    
    // Fetch car data using updated endpoint
    fetchSharedCarData(shareToken)
        .then(data => {
            renderCarData(data);
            setupShareButtons(shareToken);
        })
        .catch(error => {
            console.error("Error loading shared car:", error);
            showError("Failed to load car data");
        });
});

async function fetchSharedCarData(shareToken) {
    try {
        // Updated endpoint now uses /share/token/{share_token}/data
        const response = await fetch(`/share/token/${shareToken}/data`);
        
        if (!response.ok) {
            throw new Error(`HTTP error ${response.status}`);
        }
        
        return await response.json();
    } catch (error) {
        console.error("Failed to fetch car data:", error);
        throw error;
    }
}

function renderCarData(car) {
    // Set page title
    document.title = `${car.make} ${car.model} - CarBN`;
    
    // Update main car information
    document.getElementById('car-title').textContent = `${car.make} ${car.model}`;
    document.getElementById('car-image').src = `/images/${car.high_res_image}`;
    document.getElementById('car-image').alt = `${car.make} ${car.model} ${car.color}`;
    
    // Update likes count
    document.getElementById('likes-count').textContent = car.likes_count;
    
    // Update view count
    document.getElementById('views-count').textContent = car.view_count;
    
    // Update owner name
    document.getElementById('owner-name').textContent = car.owner_name || 'Unknown';
    
    // Update trim badge (if available)
    if (car.trim) {
        document.getElementById('trim-badge').textContent = car.trim;
        document.getElementById('trim-badge').style.display = 'flex';
    } else {
        document.getElementById('trim-badge').style.display = 'none';
    }
    
    // Update rarity badge
    if (car.rarity !== null && car.rarity !== undefined) {
        // Get the appropriate rarity text
        let rarityText;
        switch (car.rarity) {
            case 1: rarityText = "Common"; break;
            case 2: rarityText = "Uncommon"; break;
            case 3: rarityText = "Rare"; break;
            case 4: rarityText = "Epic"; break;
            case 5: rarityText = "Legendary"; break;
            default: rarityText = "Common";
        }
        
        document.getElementById('rarity-badge').textContent = rarityText;
        document.getElementById('rarity-badge').style.display = 'flex';
        
        // Add rarity class for styling (1-5)
        const rarityLevel = Math.min(Math.max(car.rarity, 1), 5);
        document.getElementById('rarity-badge').className = `badge rarity-badge rarity-${rarityLevel}`;
    } else {
        document.getElementById('rarity-badge').style.display = 'none';
    }
    
    // Update specifications
    if (car.horsepower) {
        document.getElementById('horsepower').textContent = `${car.horsepower} hp`;
    }
    
    if (car.top_speed) {
        document.getElementById('top-speed').textContent = `${car.top_speed}`;
    }
    
    if (car.acceleration) {
        document.getElementById('acceleration').textContent = `${car.acceleration} sec`;
    }
    
    // Update details
    document.getElementById('year').textContent = car.year;
    document.getElementById('trim').textContent = car.trim || 'N/A';
    
    if (car.price) {
        document.getElementById('price').textContent = formatCurrency(car.price);
    }
    
    if (car.engine_type) {
        document.getElementById('engine-type').textContent = car.engine_type;
    }
    
    if (car.drivetrain_type) {
        document.getElementById('drivetrain').textContent = car.drivetrain_type;
    }
    
    if (car.curb_weight) {
        document.getElementById('weight').textContent = `${car.curb_weight.toLocaleString()} lbs`;
    }
    
    // Update date collected in relative format
    document.getElementById('date-collected').textContent = formatRelativeDate(car.date_collected);
    
    // Update description
    if (car.description) {
        document.getElementById('description').textContent = car.description;
    }
    
    // Add owner information
    document.title = `${car.make} ${car.model} - Shared by ${car.owner_name}`;
    
    // Add custom meta tags for social sharing
    addMetaTags(car);
    
    // Update app store link with TestFlight URL
    document.getElementById('app-store-link').href = "https://apps.apple.com/ca/app/carbn/id6742416359";
}

function formatCurrency(value) {
    return new Intl.NumberFormat('en-US', {
        style: 'currency',
        currency: 'USD',
        minimumFractionDigits: 0
    }).format(value);
}

function formatRelativeDate(isoDate) {
    const date = new Date(isoDate);
    const now = new Date();
    const diffTime = Math.abs(now - date);
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));
    
    if (diffDays < 1) {
        return 'today';
    } else if (diffDays === 1) {
        return 'yesterday';
    } else if (diffDays < 7) {
        return `${diffDays} days ago`;
    } else if (diffDays < 30) {
        const weeks = Math.floor(diffDays / 7);
        return `${weeks} ${weeks === 1 ? 'week' : 'weeks'} ago`;
    } else if (diffDays < 365) {
        const months = Math.floor(diffDays / 30);
        return `${months} ${months === 1 ? 'month' : 'months'} ago`;
    } else {
        const years = Math.floor(diffDays / 365);
        return `${years} ${years === 1 ? 'year' : 'years'} ago`;
    }
}

function showError(message) {
    const container = document.querySelector('.container');
    container.innerHTML = `
        <div class="error-container">
            <h1>Oops!</h1>
            <p>${message}</p>
            <a href="/" class="back-button">Back to Home</a>
        </div>
    `;
}

function setupShareButtons(shareToken) {
    // Generate the full share URL
    const shareUrl = window.location.href;
    
    // Twitter share
    document.getElementById('share-twitter').addEventListener('click', () => {
        const text = document.getElementById('car-title').textContent;
        const url = `https://twitter.com/intent/tweet?text=${encodeURIComponent(text)}&url=${encodeURIComponent(shareUrl)}`;
        window.open(url, '_blank');
    });
    
    // Facebook share
    document.getElementById('share-facebook').addEventListener('click', () => {
        const url = `https://www.facebook.com/sharer/sharer.php?u=${encodeURIComponent(shareUrl)}`;
        window.open(url, '_blank');
    });
    
    // Copy link
    document.getElementById('copy-link').addEventListener('click', () => {
        navigator.clipboard.writeText(shareUrl)
            .then(() => {
                // Show a temporary "Copied!" message
                const button = document.getElementById('copy-link');
                const originalText = button.textContent;
                button.textContent = "Copied!";
                setTimeout(() => {
                    button.textContent = originalText;
                }, 2000);
            })
            .catch(err => {
                console.error('Failed to copy: ', err);
            });
    });
}

function addMetaTags(car) {
    // Add Open Graph and Twitter Card meta tags for better sharing
    const head = document.querySelector('head');
    
    // Open Graph tags
    const ogTags = [
        { property: "og:title", content: `${car.make} ${car.model} - CarBN` },
        { property: "og:description", content: car.description || `Check out this ${car.year} ${car.make} ${car.model} on CarBN!` },
        { property: "og:image", content: `${window.location.origin}/images/${car.high_res_image}` },
        { property: "og:url", content: window.location.href },
        { property: "og:type", content: "website" }
    ];
    
    // Twitter Card tags
    const twitterTags = [
        { name: "twitter:card", content: "summary_large_image" },
        { name: "twitter:title", content: `${car.make} ${car.model} - CarBN` },
        { name: "twitter:description", content: car.description || `Check out this ${car.year} ${car.make} ${car.model} on CarBN!` },
        { name: "twitter:image", content: `${window.location.origin}/images/${car.high_res_image}` }
    ];
    
    // Add all tags to head
    [...ogTags, ...twitterTags].forEach(tag => {
        const meta = document.createElement('meta');
        const [key, value] = Object.entries(tag)[0];
        meta.setAttribute(key, value);
        const [contentKey, contentValue] = Object.entries(tag)[1];
        meta.setAttribute(contentKey, contentValue);
        head.appendChild(meta);
    });
}