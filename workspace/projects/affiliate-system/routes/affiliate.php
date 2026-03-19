<?php

use Illuminate\Support\Facades\Route;
use App\Http\Controllers\AffiliateController;

/*
|--------------------------------------------------------------------------
| Affiliate Routes
|--------------------------------------------------------------------------
*/

// API Routes (public + protected)
Route::prefix('api/affiliate')->group(function () {
    // Public: Track clicks (redirect to target)
    Route::get('/track/{code}', [AffiliateController::class, 'track'])
        ->name('affiliate.track');

    // Protected: Create affiliate link
    Route::post('/link', [AffiliateController::class, 'createLink'])
        ->name('affiliate.link.create')
        ->middleware('auth:sanctum');

    // Protected: Get affiliate link(s)
    Route::get('/link', [AffiliateController::class, 'getLink'])
        ->name('affiliate.link.get')
        ->middleware('auth:sanctum');

    Route::get('/link/{code}', [AffiliateController::class, 'getLink'])
        ->name('affiliate.link.get.single')
        ->middleware('auth:sanctum');

    // Protected: Record conversion
    Route::post('/conversion', [AffiliateController::class, 'recordConversion'])
        ->name('affiliate.conversion')
        ->middleware('auth:sanctum');

    // Protected: Get statistics
    Route::get('/stats', [AffiliateController::class, 'getStats'])
        ->name('affiliate.stats')
        ->middleware('auth:sanctum');
});

// Web Routes (Blade views)
Route::prefix('dashboard/affiliate')->middleware(['auth', 'verified'])->group(function () {
    Route::get('/', [AffiliateController::class, 'dashboard'])
        ->name('affiliate.dashboard');

    Route::get('/links', [AffiliateController::class, 'links'])
        ->name('affiliate.links');

    Route::get('/links/create', [AffiliateController::class, 'createLinkForm'])
        ->name('affiliate.links.create');

    Route::get('/links/{code}', [AffiliateController::class, 'linkDetails'])
        ->name('affiliate.links.details');

    Route::get('/link-generator', [AffiliateController::class, 'linkGenerator'])
        ->name('affiliate.link-generator');

    Route::get('/commissions', [AffiliateController::class, 'commissions'])
        ->name('affiliate.commissions');

    Route::get('/conversions', [AffiliateController::class, 'conversions'])
        ->name('affiliate.conversions');
});
