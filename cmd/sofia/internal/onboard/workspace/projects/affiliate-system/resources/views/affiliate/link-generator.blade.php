@extends('layouts.app')

@section('title', 'Affiliate Link Generator')

@section('content')
<div class="container mx-auto px-4 py-8">
    <div class="mb-8">
        <a href="{{ route('affiliate.dashboard') }}" class="inline-flex items-center text-gray-600 hover:text-gray-900 mb-4">
            <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
            </svg>
            Back to Dashboard
        </a>
        <h1 class="text-3xl font-bold text-gray-900">Affiliate Link Generator</h1>
        <p class="text-gray-600 mt-2">Create tracking links to promote products and earn commissions</p>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-8">
        <!-- Create Link Form -->
        <div class="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <h2 class="text-lg font-semibold text-gray-900 mb-6">Create New Link</h2>
            
            <form id="link-form" class="space-y-6">
                @csrf
                
                <!-- Link Name -->
                <div>
                    <label for="name" class="block text-sm font-medium text-gray-700 mb-2">Link Name</label>
                    <input type="text" id="name" name="name" required
                        class="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
                        placeholder="e.g., Summer Sale Campaign">
                </div>

                <!-- Target URL -->
                <div>
                    <label for="target_url" class="block text-sm font-medium text-gray-700 mb-2">Target URL</label>
                    <input type="url" id="target_url" name="target_url" required
                        class="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
                        placeholder="https://example.com/product">
                </div>

                <!-- Commission Type -->
                <div>
                    <label class="block text-sm font-medium text-gray-700 mb-2">Commission Type</label>
                    <div class="flex gap-4">
                        <label class="flex items-center">
                            <input type="radio" name="commission_type" value="percentage" checked
                                class="w-4 h-4 text-indigo-600 border-gray-300 focus:ring-indigo-500">
                            <span class="ml-2 text-sm text-gray-600">Percentage (%)</span>
                        </label>
                        <label class="flex items-center">
                            <input type="radio" name="commission_type" value="fixed"
                                class="w-4 h-4 text-indigo-600 border-gray-300 focus:ring-indigo-500">
                            <span class="ml-2 text-sm text-gray-600">Fixed Amount (kr)</span>
                        </label>
                    </div>
                </div>

                <!-- Commission Rate -->
                <div id="percentage-field">
                    <label for="commission_rate" class="block text-sm font-medium text-gray-700 mb-2">Commission Rate (%)</label>
                    <input type="number" id="commission_rate" name="commission_rate" min="0" max="100" step="0.01" value="10"
                        class="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500">
                </div>

                <div id="fixed-field" class="hidden">
                    <label for="commission_fixed" class="block text-sm font-medium text-gray-700 mb-2">Fixed Commission (kr)</label>
                    <input type="number" id="commission_fixed" name="commission_fixed" min="0" step="0.01"
                        class="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
                        placeholder="e.g., 50.00">
                </div>

                <!-- Advanced Options -->
                <details class="group">
                    <summary class="flex items-center cursor-pointer text-sm font-medium text-gray-700 hover:text-gray-900">
                        <svg class="w-4 h-4 mr-2 transition-transform group-open:rotate-90" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                        </svg>
                        Advanced Options
                    </summary>
                    <div class="mt-4 space-y-4 pl-6">
                        <!-- Max Conversions -->
                        <div>
                            <label for="max_conversions" class="block text-sm font-medium text-gray-700 mb-2">Max Conversions (optional)</label>
                            <input type="number" id="max_conversions" name="max_conversions" min="1"
                                class="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
                                placeholder="Leave empty for unlimited">
                        </div>

                        <!-- Start Date -->
                        <div>
                            <label for="starts_at" class="block text-sm font-medium text-gray-700 mb-2">Start Date (optional)</label>
                            <input type="datetime-local" id="starts_at" name="starts_at"
                                class="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500">
                        </div>

                        <!-- Expiration Date -->
                        <div>
                            <label for="expires_at" class="block text-sm font-medium text-gray-700 mb-2">Expiration Date (optional)</label>
                            <input type="datetime-local" id="expires_at" name="expires_at"
                                class="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500">
                        </div>
                    </div>
                </details>

                <!-- Submit -->
                <button type="submit" id="submit-btn"
                    class="w-full px-6 py-3 bg-indigo-600 text-white font-medium rounded-lg hover:bg-indigo-700 transition-colors flex items-center justify-center">
                    <span id="btn-text">Create Affiliate Link</span>
                    <svg id="btn-loading" class="hidden w-5 h-5 ml-2 animate-spin" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                </button>
            </form>

            <!-- Error Message -->
            <div id="error-message" class="hidden mt-4 p-4 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm"></div>
        </div>

        <!-- Result Card -->
        <div class="space-y-6">
            <!-- Generated Link -->
            <div id="result-card" class="bg-white rounded-xl shadow-sm border border-gray-100 p-6 hidden">
                <h2 class="text-lg font-semibold text-gray-900 mb-4">Your Affiliate Link</h2>
                
                <div class="mb-4">
                    <label class="block text-sm font-medium text-gray-700 mb-2">Tracking URL</label>
                    <div class="flex gap-2">
                        <input type="text" id="generated-url" readonly
                            class="flex-1 px-4 py-2 bg-gray-50 border border-gray-300 rounded-lg text-sm font-mono">
                        <button onclick="copyToClipboard()" class="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 transition-colors">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                            </svg>
                        </button>
                    </div>
                    <p class="text-xs text-gray-500 mt-1">Share this link to start tracking clicks and earn commissions</p>
                </div>

                <!-- Share Options -->
                <div class="border-t border-gray-100 pt-4">
                    <label class="block text-sm font-medium text-gray-700 mb-3">Share via</label>
                    <div class="flex gap-3">
                        <a id="share-twitter" target="_blank"
                            class="w-10 h-10 bg-blue-400 text-white rounded-lg flex items-center justify-center hover:bg-blue-500 transition-colors">
                            <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
                                <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/>
                            </svg>
                        </a>
                        <a id="share-facebook" target="_blank"
                            class="w-10 h-10 bg-blue-600 text-white rounded-lg flex items-center justify-center hover:bg-blue-700 transition-colors">
                            <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
                                <path d="M24 12.073c0-6.627-5.373-12-12-12s-12 5.373-12 12c0 5.99 4.388 10.954 10.125 11.854v-8.385H7.078v-3.47h3.047V9.43c0-3.007 1.792-4.669 4.533-4.669 1.312 0 2.686.235 2.686.235v2.953H15.83c-1.491 0-1.956.925-1.956 1.874v2.25h3.328l-.532 3.47h-2.796v8.385C19.612 23.027 24 18.062 24 12.073z"/>
                            </svg>
                        </a>
                        <a id="share-linkedin" target="_blank"
                            class="w-10 h-10 bg-blue-700 text-white rounded-lg flex items-center justify-center hover:bg-blue-800 transition-colors">
                            <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
                                <path d="M20.447 20.452h-3.554v-5.569c0-1.328-.027-3.037-1.852-3.037-1.853 0-2.136 1.445-2.136 2.939v5.667H9.351V9h3.414v1.561h.046c.477-.9 1.637-1.85 3.37-1.85 3.601 0 4.267 2.37 4.267 5.455v6.286zM5.337 7.433c-1.144 0-2.063-.926-2.063-2.065 0-1.138.92-2.063 2.063-2.063 1.14 0 2.064.925 2.064 2.063 0 1.139-.925 2.065-2.064 2.065zm1.782 13.019H3.555V9h3.564v11.452zM22.225 0H1.771C.792 0 0 .774 0 1.729v20.542C0 23.227.792 24 1.771 24h20.451C23.2 24 24 23.227 24 22.271V1.729C24 .774 23.2 0 22.222 0h.003z"/>
                            </svg>
                        </a>
                        <a id="share-email" target="_blank"
                            class="w-10 h-10 bg-gray-600 text-white rounded-lg flex items-center justify-center hover:bg-gray-700 transition-colors">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                            </svg>
                        </a>
                    </div>
                </div>

                <!-- Copy Success -->
                <div id="copy-success" class="hidden mt-4 p-3 bg-green-50 border border-green-200 rounded-lg text-green-700 text-sm flex items-center">
                    <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                    </svg>
                    Link copied to clipboard!
                </div>
            </div>

            <!-- Tracking Code -->
            <div class="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
                <h2 class="text-lg font-semibold text-gray-900 mb-4">Website Tracking Code</h2>
                <p class="text-sm text-gray-600 mb-4">Add this script to your website to automatically track affiliate clicks:</p>
                
                <div class="bg-gray-900 rounded-lg p-4 overflow-x-auto">
                    <pre class="text-sm text-gray-100 font-mono"><code id="tracking-script">&lt;script&gt;
(function() {
  var script = document.createElement('script');
  script.async = true;
  script.src = '{{ config("app.url") }}/js/affiliate-tracker.js';
  script.onload = function() {
    window.AffiliateTracker.init({
      debug: false,
      cookieDays: 30
    });
  };
  document.head.appendChild(script);
})();
&lt;/script&gt;</code></pre>
                </div>
                <button onclick="copyScriptToClipboard()" class="mt-4 px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 transition-colors flex items-center">
                    <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                    </svg>
                    Copy Script
                </button>
            </div>
        </div>
    </div>
</div>
@endsection

@push('scripts')
<script>
document.addEventListener('DOMContentLoaded', function() {
    // Commission type toggle
    const percentageField = document.getElementById('percentage-field');
    const fixedField = document.getElementById('fixed-field');
    const commissionRadios = document.querySelectorAll('input[name="commission_type"]');
    
    commissionRadios.forEach(radio => {
        radio.addEventListener('change', function() {
            if (this.value === 'percentage') {
                percentageField.classList.remove('hidden');
                fixedField.classList.add('hidden');
            } else {
                percentageField.classList.add('hidden');
                fixedField.classList.remove('hidden');
            }
        });
    });

    // Form submission
    const form = document.getElementById('link-form');
    const submitBtn = document.getElementById('submit-btn');
    const btnText = document.getElementById('btn-text');
    const btnLoading = document.getElementById('btn-loading');
    const errorMessage = document.getElementById('error-message');
    const resultCard = document.getElementById('result-card');

    form.addEventListener('submit', async function(e) {
        e.preventDefault();
        
        // Show loading
        btnText.textContent = 'Creating...';
        btnLoading.classList.remove('hidden');
        submitBtn.disabled = true;
        errorMessage.classList.add('hidden');

        const formData = new FormData(form);
        const data = Object.fromEntries(formData);

        try {
            const response = await fetch('/api/affiliate/link', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-TOKEN': document.querySelector('meta[name="csrf-token"]')?.content,
                    'Accept': 'application/json'
                },
                body: JSON.stringify(data)
            });

            const result = await response.json();

            if (result.success) {
                // Show result
                document.getElementById('generated-url').value = result.tracking_url;
                resultCard.classList.remove('hidden');
                
                // Update share links
                const url = encodeURIComponent(result.tracking_url);
                document.getElementById('share-twitter').href = `https://twitter.com/intent/tweet?url=${url}`;
                document.getElementById('share-facebook').href = `https://www.facebook.com/sharer/sharer.php?u=${url}`;
                document.getElementById('share-linkedin').href = `https://www.linkedin.com/sharing/share-offsite/?url=${url}`;
                document.getElementById('share-email').href = `mailto:?subject=Check%20this%20out&body=${url}`;

                // Reset form
                form.reset();
            } else {
                errorMessage.textContent = result.message || 'Failed to create link';
                errorMessage.classList.remove('hidden');
            }
        } catch (error) {
            errorMessage.textContent = 'An error occurred. Please try again.';
            errorMessage.classList.remove('hidden');
        } finally {
            btnText.textContent = 'Create Affiliate Link';
            btnLoading.classList.add('hidden');
            submitBtn.disabled = false;
        }
    });
});

function copyToClipboard() {
    const url = document.getElementById('generated-url').value;
    navigator.clipboard.writeText(url).then(() => {
        const success = document.getElementById('copy-success');
        success.classList.remove('hidden');
        setTimeout(() => success.classList.add('hidden'), 3000);
    });
}

function copyScriptToClipboard() {
    const script = document.getElementById('tracking-script').textContent;
    navigator.clipboard.writeText(script);
}
</script>
@endpush
