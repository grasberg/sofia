// Hero Component JavaScript
(function() {
  class HeroComponent {
    constructor(container) {
      this.container = container;
      this.init();
    }

    init() {
      // Add intersection observer for animation
      if ('IntersectionObserver' in window) {
        const observer = new IntersectionObserver((entries) => {
          entries.forEach(entry => {
            if (entry.isIntersecting) {
              entry.target.classList.add('lp-hero--visible');
            }
          });
        }, { threshold: 0.1 });

        observer.observe(this.container);
      }

      // Initialize any interactive elements
      this.initVideo();
    }

    initVideo() {
      const video = this.container.querySelector('.lp-hero__video');
      if (video) {
        // Ensure video plays when in viewport
        const videoObserver = new IntersectionObserver((entries) => {
          entries.forEach(entry => {
            if (entry.isIntersecting) {
              video.play().catch(e => console.log('Video autoplay failed:', e));
            } else {
              video.pause();
            }
          });
        }, { threshold: 0.5 });

        videoObserver.observe(video);
      }
    }
  }

  // Initialize hero components when DOM is loaded
  document.addEventListener('DOMContentLoaded', () => {
    const heroElements = document.querySelectorAll('.lp-hero');
    heroElements.forEach(hero => {
      new HeroComponent(hero);
    });
  });

  // Export for module systems if needed
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = HeroComponent;
  }
})();