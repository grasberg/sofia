// Sofia Landing Page JavaScript

// Theme toggle
const themeToggle = document.getElementById('themeToggle');
const html = document.documentElement;
const savedTheme = localStorage.getItem('theme') || 'dark';
html.setAttribute('data-theme', savedTheme);
themeToggle.textContent = savedTheme === 'dark' ? '☀️' : '🌙';

themeToggle.addEventListener('click', () => {
    const current = html.getAttribute('data-theme');
    const next = current === 'dark' ? 'light' : 'dark';
    html.setAttribute('data-theme', next);
    localStorage.setItem('theme', next);
    themeToggle.textContent = next === 'dark' ? '☀️' : '🌙';
});

// Mobile menu
const mobileToggle = document.getElementById('mobileToggle');
const navLinks = document.getElementById('navLinks');
mobileToggle.addEventListener('click', () => { navLinks.classList.toggle('active'); });
navLinks.querySelectorAll('a').forEach(link => {
    link.addEventListener('click', () => { navLinks.classList.remove('active'); });
});

// Copy button
document.querySelectorAll('.copy-btn').forEach(btn => {
    btn.addEventListener('click', () => {
        const code = btn.getAttribute('data-code');
        navigator.clipboard.writeText(code).then(() => {
            btn.textContent = 'Copied!';
            btn.classList.add('copied');
            setTimeout(() => { btn.textContent = 'Copy'; btn.classList.remove('copied'); }, 2000);
        });
    });
});

// Score bar animation
const animateScoreBars = () => {
    document.querySelectorAll('.score-bar').forEach(bar => {
        const target = bar.getAttribute('data-width');
        bar.style.width = target + '%';
    });
};

// Intersection observer for animations
const observer = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
        if (entry.isIntersecting) {
            entry.target.classList.add('visible');
        }
    });
}, { threshold: 0.1, rootMargin: '0px 0px -50px 0px' });

document.querySelectorAll('.feature-card, .community-card, .score-card, .code-block, .step-card').forEach(el => {
    el.classList.add('fade-in');
    observer.observe(el);
});

// Scorecard bar animation on scroll
const scorecardSection = document.querySelector('.scorecard');
if (scorecardSection) {
    const scoreObserver = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                animateScoreBars();
                scoreObserver.unobserve(entry.target);
            }
        });
    }, { threshold: 0.2 });
    scoreObserver.observe(scorecardSection);
}

// Smooth scroll for anchor links
document.querySelectorAll('a[href^="#"]').forEach(anchor => {
    anchor.addEventListener('click', (e) => {
        e.preventDefault();
        const target = document.querySelector(anchor.getAttribute('href'));
        if (target) { target.scrollIntoView({ behavior: 'smooth', block: 'start' }); }
    });
});