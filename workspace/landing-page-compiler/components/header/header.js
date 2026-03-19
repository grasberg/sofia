// Header Component JavaScript
(function() {
  class HeaderComponent {
    constructor(container) {
      this.container = container;
      this.mobileToggle = this.container.querySelector('.lp-header__mobile-toggle');
      this.nav = this.container.querySelector('.lp-header__nav');
      this.init();
    }

    init() {
      if (this.mobileToggle) {
        this.mobileToggle.addEventListener('click', () => this.toggleMobileMenu());
      }

      // Close mobile menu when clicking on a link
      const menuLinks = this.container.querySelectorAll('.lp-header__menu-link');
      menuLinks.forEach(link => {
        link.addEventListener('click', () => {
          if (window.innerWidth <= 768) {
            this.closeMobileMenu();
          }
        });
      });

      // Close mobile menu on window resize
      window.addEventListener('resize', () => {
        if (window.innerWidth > 768) {
          this.closeMobileMenu();
        }
      });

      // Close mobile menu when clicking outside
      document.addEventListener('click', (event) => {
        if (window.innerWidth <= 768 && 
            this.nav.classList.contains('lp-header__nav--open') &&
            !this.container.contains(event.target)) {
          this.closeMobileMenu();
        }
      });
    }

    toggleMobileMenu() {
      const isOpen = this.nav.classList.contains('lp-header__nav--open');
      
      if (isOpen) {
        this.closeMobileMenu();
      } else {
        this.openMobileMenu();
      }
    }

    openMobileMenu() {
      this.nav.classList.add('lp-header__nav--open');
      this.mobileToggle.classList.add('lp-header__mobile-toggle--open');
      document.body.style.overflow = 'hidden';
    }

    closeMobileMenu() {
      this.nav.classList.remove('lp-header__nav--open');
      this.mobileToggle.classList.remove('lp-header__mobile-toggle--open');
      document.body.style.overflow = '';
    }
  }

  // Initialize header components when DOM is loaded
  document.addEventListener('DOMContentLoaded', () => {
    const headerElements = document.querySelectorAll('.lp-header');
    headerElements.forEach(header => {
      new HeaderComponent(header);
    });
  });

  // Export for module systems if needed
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = HeaderComponent;
  }
})();