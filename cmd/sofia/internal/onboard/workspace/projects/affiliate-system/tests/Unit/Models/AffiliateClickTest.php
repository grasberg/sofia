<?php

namespace Tests\Unit\Models;

use Tests\TestCase;
use App\Models\AffiliateClick;
use App\Models\AffiliateLink;
use App\Models\User;
use Illuminate\Foundation\Testing\RefreshDatabase;

class AffiliateClickTest extends TestCase
{
    use RefreshDatabase;

    /** @test */
    public function it_can_create_affiliate_click()
    {
        // Arrange
        $link = AffiliateLink::factory()->create();

        // Act
        $click = AffiliateClick::create([
            'affiliate_link_id' => $link->id,
            'user_id' => $link->user_id,
            'ip_address' => '192.168.1.1',
            'user_agent' => 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36',
            'referer' => 'https://google.com',
            'country_code' => 'SE',
            'device_type' => 'desktop',
            'browser' => 'Chrome',
            'os' => 'Windows',
            'utm_data' => ['utm_source' => 'google'],
            'session_id' => 'session123',
            'clicked_at' => now(),
        ]);

        // Assert
        $this->assertInstanceOf(AffiliateClick::class, $click);
        $this->assertEquals($link->id, $click->affiliate_link_id);
        $this->assertEquals('192.168.1.1', $click->ip_address);
        $this->assertEquals('SE', $click->country_code);
        $this->assertEquals('desktop', $click->device_type);
        $this->assertEquals('Chrome', $click->browser);
        $this->assertEquals('Windows', $click->os);
        $this->assertEquals(['utm_source' => 'google'], $click->utm_data);
    }

    /** @test */
    public function it_belongs_to_affiliate_link()
    {
        // Arrange
        $link = AffiliateLink::factory()->create();
        $click = AffiliateClick::factory()->create(['affiliate_link_id' => $link->id]);

        // Act & Assert
        $this->assertTrue($click->affiliateLink->is($link));
    }

    /** @test */
    public function it_belongs_to_user()
    {
        // Arrange
        $user = User::factory()->create();
        $click = AffiliateClick::factory()->create(['user_id' => $user->id]);

        // Act & Assert
        $this->assertTrue($click->user->is($user));
    }

    /** @test */
    public function it_has_conversions_relationship()
    {
        // Arrange
        $click = AffiliateClick::factory()->create();
        $conversion = \App\Models\AffiliateConversion::factory()->create([
            'affiliate_click_id' => $click->id,
        ]);

        // Refresh relationship
        $click->refresh();

        // Act & Assert
        $this->assertTrue($click->conversions->contains($conversion));
    }

    /** @test */
    public function it_can_scope_by_device_type()
    {
        // Arrange
        $desktopClick = AffiliateClick::factory()->desktop()->create();
        $mobileClick = AffiliateClick::factory()->mobile()->create();

        // Act
        $desktopClicks = AffiliateClick::where('device_type', 'desktop')->get();
        $mobileClicks = AffiliateClick::where('device_type', 'mobile')->get();

        // Assert
        $this->assertTrue($desktopClicks->contains($desktopClick));
        $this->assertFalse($desktopClicks->contains($mobileClick));
        $this->assertTrue($mobileClicks->contains($mobileClick));
        $this->assertFalse($mobileClicks->contains($desktopClick));
    }

    /** @test */
    public function it_can_scope_by_country()
    {
        // Arrange
        $swedishClick = AffiliateClick::factory()->country('SE')->create();
        $usClick = AffiliateClick::factory()->country('US')->create();

        // Act
        $swedishClicks = AffiliateClick::where('country_code', 'SE')->get();
        $usClicks = AffiliateClick::where('country_code', 'US')->get();

        // Assert
        $this->assertTrue($swedishClicks->contains($swedishClick));
        $this->assertFalse($swedishClicks->contains($usClick));
        $this->assertTrue($usClicks->contains($usClick));
        $this->assertFalse($usClicks->contains($swedishClick));
    }

    /** @test */
    public function it_casts_utm_data_to_array()
    {
        // Arrange
        $click = AffiliateClick::factory()->create([
            'utm_data' => ['utm_source' => 'facebook', 'utm_medium' => 'social'],
        ]);

        // Refresh from database
        $click->refresh();

        // Act & Assert
        $this->assertIsArray($click->utm_data);
        $this->assertEquals('facebook', $click->utm_data['utm_source']);
        $this->assertEquals('social', $click->utm_data['utm_medium']);
    }

    /** @test */
    public function it_returns_correct_click_age_in_days()
    {
        // Arrange
        $click = AffiliateClick::factory()->create([
            'clicked_at' => now()->subDays(5),
        ]);

        // Act
        $ageInDays = $click->clicked_at->diffInDays();

        // Assert
        $this->assertEquals(5, $ageInDays);
    }
}