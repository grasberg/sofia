/**
 * Affiliate Tracker Script
 * 
 * Automatically tracks affiliate clicks and stores them in cookies
 * for later conversion attribution.
 * 
 * Usage:
 * <script src="/js/affiliate-tracker.js"></script>
 * <script>
 *   window.AffiliateTracker.init({
 *     debug: false,
 *     cookieDays: 30,
 *     apiUrl: '/api/affiliate'
 *   });
 * </script>
 */
(function(window, document) {
    'use strict';

    const AffiliateTracker = {
        config: {
            debug: false,
            cookieDays: 30,
            cookieName: 'affiliate_click',
            apiUrl: '',
            storageType: 'cookie' // 'cookie' or 'localStorage'
        },

        /**
         * Initialize the tracker
         */
        init: function(options) {
            this.config = { ...this.config, ...options };
            this.log('Affiliate Tracker initialized');

            // Check for affiliate click in URL
            this.checkUrlForAffiliate();

            // Attach click listeners to affiliate links
            this.attachLinkListeners();

            // Handle page unload to cleanup
            window.addEventListener('beforeunload', this.cleanup.bind(this));
        },

        /**
         * Log debug messages
         */
        log: function(...args) {
            if (this.config.debug && window.console) {
                console.log('[AffiliateTracker]', ...args);
            }
        },

        /**
         * Check URL for affiliate parameters
         */
        checkUrlForAffiliate: function() {
            const urlParams = new URLSearchParams(window.location.search);
            const affiliateClick = urlParams.get('affiliate_click');
            const affiliateCode = urlParams.get('affiliate_code');

            if (affiliateClick) {
                this.log('Found affiliate click ID:', affiliateClick);
                this.storeAffiliateData({
                    clickId: affiliateClick,
                    code: affiliateCode,
                    timestamp: Date.now()
                });
            }
        },

        /**
         * Store affiliate data in cookie or localStorage
         */
        storeAffiliateData: function(data) {
            const storageData = JSON.stringify({
                ...data,
                expires: Date.now() + (this.config.cookieDays * 24 * 60 * 60 * 1000)
            });

            if (this.config.storageType === 'localStorage') {
                try {
                    localStorage.setItem(this.config.cookieName, storageData);
                    this.log('Stored in localStorage:', data);
                } catch (e) {
                    this.log('localStorage full, falling back to cookie');
                    this.storeInCookie(storageData);
                }
            } else {
                this.storeInCookie(storageData);
            }
        },

        /**
         * Store data in cookie
         */
        storeInCookie: function(data) {
            const expires = new Date();
            expires.setTime(expires.getTime() + (this.config.cookieDays * 24 * 60 * 60 * 1000));
            document.cookie = this.config.cookieName + '=' + encodeURIComponent(data) +
                ';expires=' + expires.toUTCString() +
                ';path=/' +
                ';SameSite=Lax';
            this.log('Stored in cookie');
        },

        /**
         * Get stored affiliate data
         */
        getAffiliateData: function() {
            let data;

            if (this.config.storageType === 'localStorage') {
                data = localStorage.getItem(this.config.cookieName);
            }

            if (!data) {
                data = this.getCookie(this.config.cookieName);
            }

            if (!data) return null;

            try {
                const parsed = JSON.parse(decodeURIComponent(data));
                
                // Check if expired
                if (parsed.expires && Date.now() > parsed.expires) {
                    this.clearAffiliateData();
                    return null;
                }

                return parsed;
            } catch (e) {
                this.log('Failed to parse affiliate data');
                return null;
            }
        },

        /**
         * Get cookie by name
         */
        getCookie: function(name) {
            const value = '; ' + document.cookie;
            const parts = value.split('; ' + name + '=');
            if (parts.length === 2) {
                return parts.pop().split(';').shift();
            }
            return null;
        },

        /**
         * Clear stored affiliate data
         */
        clearAffiliateData: function() {
            if (this.config.storageType === 'localStorage') {
                localStorage.removeItem(this.config.cookieName);
            }
            document.cookie = this.config.cookieName + '=;expires=Thu, 01 Jan 1970 00:00:00 UTC;path=/;';
            this.log('Cleared affiliate data');
        },

        /**
         * Attach click listeners to affiliate links
         */
        attachLinkListeners: function() {
            const links = document.querySelectorAll('a[href*="affiliate_code"], a[data-affiliate]');
            
            links.forEach(link => {
                link.addEventListener('click', (e) => {
                    const code = link.dataset.affiliate || this.getAffiliateCodeFromHref(link.href);
                    if (code) {
                        this.trackLinkClick(code, link.href);
                    }
                });
            });
        },

        /**
         * Extract affiliate code from href
         */
        getAffiliateCodeFromHref: function(href) {
            try {
                const url = new URL(href);
                return url.searchParams.get('affiliate_code');
            } catch {
                return null;
            }
        },

        /**
         * Track a click on an affiliate link
         */
        trackLinkClick: function(code, targetUrl) {
            this.log('Tracking affiliate click:', code);

            // Send click to server
            if (this.config.apiUrl) {
                fetch(this.config.apiUrl + '/click', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'X-CSRF-TOKEN': this.getCsrfToken()
                    },
                    body: JSON.stringify({
                        code: code,
                        url: targetUrl,
                        referer: document.referrer,
                        user_agent: navigator.userAgent
                    }),
                    keepalive: true
                }).catch(err => {
                    this.log('Failed to track click:', err);
                });
            }
        },

        /**
         * Record a conversion
         */
        trackConversion: function(conversionData) {
            const affiliateData = this.getAffiliateData();

            if (!affiliateData && !conversionData.code) {
                this.log('No affiliate data found for conversion');
                return Promise.reject(new Error('No affiliate data'));
            }

            const data = {
                ...conversionData,
                click_id: affiliateData?.clickId || null,
                affiliate_code: affiliateData?.code || conversionData.code,
                conversion_type: conversionData.type || 'sale',
                order_value: conversionData.orderValue || 0
            };

            this.log('Tracking conversion:', data);

            return fetch(this.config.apiUrl + '/conversion', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-TOKEN': this.getCsrfToken()
                },
                body: JSON.stringify(data)
            }).then(response => response.json());
        },

        /**
         * Get CSRF token from meta tag or page
         */
        getCsrfToken: function() {
            const meta = document.querySelector('meta[name="csrf-token"]');
            return meta ? meta.content : '';
        },

        /**
         * Cleanup expired data
         */
        cleanup: function() {
            const data = this.getAffiliateData();
            if (data && data.expires && Date.now() > data.expires) {
                this.clearAffiliateData();
            }
        },

        /**
         * Get current affiliate data (for external use)
         */
        getData: function() {
            return this.getAffiliateData();
        },

        /**
         * Has affiliate data
         */
        hasAffiliateData: function() {
            return this.getAffiliateData() !== null;
        },

        /**
         * Create a conversion helper function
         * Call this when a conversion happens (purchase, signup, etc.)
         */
        convert: function(options = {}) {
            return this.trackConversion({
                type: options.type || 'sale',
                orderValue: options.value || options.orderValue || 0,
                orderId: options.orderId || options.transactionId || null,
                metadata: options.metadata || {}
            });
        }
    };

    // Expose globally
    window.AffiliateTracker = AffiliateTracker;

    // Auto-init if data attribute is present
    if (document.currentScript && document.currentScript.dataset.autoInit !== 'false') {
        document.addEventListener('DOMContentLoaded', function() {
            AffiliateTracker.init({});
        });
    }

})(window, document);
