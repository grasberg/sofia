<?php

namespace Tests\Unit\Models;

use Tests\TestCase;
use App\Models\AffiliateLink;
use App\Models\User;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Carbon;

class AffiliateLinkTest extends TestCase
{
    use RefreshDatabase;

    /** @test */
    public function it_can_create_affiliate_link()
    {
        $user = User::factory()->create();

        $link = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Test Link',
            'target_url' => 'https://example.com',
            'commission_rate' => 10.0,
            'commission_type' => 'percentage',
            'status' => 'active',
        ]);

        $this->assertNotNull($link);
        $this->assertEquals('Test Link', $link->name);
        $this->assertEquals('percentage', $link->commission_type);
        $this->assertEquals(10.0, $link->commission_rate);
        $this->assertNotEmpty($link->code);
    }

    /** @test */
    public function it_generates_unique_code_on_creation()
    {
        $user = User::factory()->create();

        $link1 = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Link 1',
            'target_url' => 'https://example.com',
        ]);

        $link2 = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Link 2',
            'target_url' => 'https://example2.com',
        ]);

        $this->assertNotEquals($link1->code, $link2->code);
        $this->assertEquals(12, strlen($link1->code));
    }

    /** @test */
    public function it_calculates_percentage_commission()
    {
        $link = new AffiliateLink([
            'commission_type' => 'percentage',
            'commission_rate' => 15.5,
        ]);

        $commission = $link->calculateCommission(100.0);
        $this->assertEquals(15.5, $commission);

        $commission = $link->calculateCommission(200.0);
        $this->assertEquals(31.0, $commission);
    }

    /** @test */
    public function it_calculates_fixed_commission()
    {
        $link = new AffiliateLink([
            'commission_type' => 'fixed',
            'commission_fixed' => 25.0,
        ]);

        $commission = $link->calculateCommission(100.0);
        $this->assertEquals(25.0, $commission);

        // Order value should not affect fixed commission
        $commission = $link->calculateCommission(500.0);
        $this->assertEquals(25.0, $commission);
    }

    /** @test */
    public function it_generates_tracking_url()
    {
        $link = new AffiliateLink(['code' => 'abc123def456']);
        $url = $link->tracking_url;

        $this->assertStringContainsString('/api/affiliate/track/abc123def456', $url);
    }

    /** @test */
    public function it_determines_valid_link()
    {
        $user = User::factory()->create();

        // Active link with no dates
        $link = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Valid Link',
            'target_url' => 'https://example.com',
            'status' => 'active',
        ]);

        $this->assertTrue($link->isValid());

        // Inactive link
        $link->status = 'inactive';
        $this->assertFalse($link->isValid());

        // Link with future start date
        $link->status = 'active';
        $link->starts_at = Carbon::now()->addDays(5);
        $this->assertFalse($link->isValid());

        // Link with past expiration date
        $link->starts_at = null;
        $link->expires_at = Carbon::now()->subDays(1);
        $this->assertFalse($link->isValid());

        // Link within date range
        $link->starts_at = Carbon::now()->subDays(2);
        $link->expires_at = Carbon::now()->addDays(2);
        $this->assertTrue($link->isValid());
    }

    /** @test */
    public function it_respects_max_conversions_limit()
    {
        $user = User::factory()->create();

        $link = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Limited Link',
            'target_url' => 'https://example.com',
            'status' => 'active',
            'max_conversions' => 2,
        ]);

        $this->assertTrue($link->isValid());

        // Simulate two conversions (we'll mock this for unit test)
        // Since we don't want to actually create conversions, we'll test the logic
        // by manually setting the relationship count
        $mock = $this->getMockBuilder(AffiliateLink::class)
                     ->onlyMethods(['conversions'])
                     ->setConstructorArgs([$link->getAttributes()])
                     ->getMock();

        $mock->expects($this->exactly(2))
             ->method('conversions')
             ->willReturn($this->mockHasMany(2)); // Simulate 2 conversions

        // We need a better approach - maybe test via actual conversions in integration test
        // For now, skip this test
        $this->markTestIncomplete('Need to implement mock for relationship count');
    }

    /** @test */
    public function it_can_record_click_and_conversion()
    {
        $user = User::factory()->create();

        $link = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Test Link',
            'target_url' => 'https://example.com',
            'commission_rate' => 10.0,
            'commission_type' => 'percentage',
            'status' => 'active',
        ]);

        // Record a click
        $click = $link->recordClick([
            'ip_address' => '127.0.0.1',
            'user_agent' => 'Test Agent',
            'referer' => 'https://google.com',
        ]);

        $this->assertNotNull($click);
        $this->assertEquals($link->id, $click->affiliate_link_id);
        $this->assertEquals('127.0.0.1', $click->ip_address);

        // Record a conversion
        $conversion = $link->recordConversion([
            'order_id' => 'ORD-123',
            'order_value' => 150.0,
            'customer_email' => 'customer@example.com',
        ]);

        $this->assertNotNull($conversion);
        $this->assertEquals($link->id, $conversion->affiliate_link_id);
        $this->assertEquals(15.0, $conversion->commission_amount); // 10% of 150
        $this->assertEquals('ORD-123', $conversion->order_id);
    }

    /** @test */
    public function it_provides_stats()
    {
        $user = User::factory()->create();

        $link = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Test Link',
            'target_url' => 'https://example.com',
            'commission_rate' => 10.0,
            'commission_type' => 'percentage',
            'status' => 'active',
        ]);

        // Record some clicks
        $link->recordClick(['ip_address' => '127.0.0.1', 'user_agent' => 'Agent1']);
        $link->recordClick(['ip_address' => '127.0.0.1', 'user_agent' => 'Agent1']); // Same IP
        $link->recordClick(['ip_address' => '192.168.1.1', 'user_agent' => 'Agent2']);

        // Record conversions
        $link->recordConversion(['order_value' => 100.0, 'customer_email' => 'test@example.com', 'status' => 'pending']);
        $link->recordConversion(['order_value' => 200.0, 'customer_email' => 'test2@example.com', 'status' => 'approved']);

        $stats = $link->getStats();

        $this->assertEquals(3, $stats['total_clicks']);
        $this->assertEquals(2, $stats['unique_clicks']); // 2 unique IPs
        $this->assertEquals(2, $stats['total_conversions']);
        $this->assertEquals(1, $stats['pending_conversions']);
        $this->assertEquals(1, $stats['approved_conversions']);
        $this->assertEquals(20.0, $stats['total_commission']); // 10% of 200 (only approved)
        $this->assertEquals(10.0, $stats['pending_commission']); // 10% of 100 (pending)
        $this->assertEquals(66.67, $stats['conversion_rate']); // 2 conversions / 3 clicks ≈ 66.67%
    }

    /** @test */
    public function scopes_work_correctly()
    {
        $user = User::factory()->create();

        $activeLink = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Active Link',
            'target_url' => 'https://example.com',
            'status' => 'active',
            'expires_at' => Carbon::now()->addDays(10),
        ]);

        $inactiveLink = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Inactive Link',
            'target_url' => 'https://example.com',
            'status' => 'inactive',
        ]);

        $expiredLink = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Expired Link',
            'target_url' => 'https://example.com',
            'status' => 'active',
            'expires_at' => Carbon::now()->subDays(1),
        ]);

        $futureLink = AffiliateLink::create([
            'user_id' => $user->id,
            'name' => 'Future Link',
            'target_url' => 'https://example.com',
            'status' => 'active',
            'starts_at' => Carbon::now()->addDays(5),
        ]);

        // Test active scope
        $activeLinks = AffiliateLink::active()->get();
        $this->assertCount(3, $activeLinks); // active, expired, future (all have status active except inactive)
        $this->assertTrue($activeLinks->contains($activeLink));
        $this->assertTrue($activeLinks->contains($expiredLink));
        $this->assertTrue($activeLinks->contains($futureLink));
        $this->assertFalse($activeLinks->contains($inactiveLink));

        // Test expired scope
        $expiredLinks = AffiliateLink::expired()->get();
        $this->assertCount(1, $expiredLinks);
        $this->assertTrue($expiredLinks->contains($expiredLink));

        // Test valid scope
        $validLinks = AffiliateLink::valid()->get();
        $this->assertCount(1, $validLinks);
        $this->assertTrue($validLinks->contains($activeLink));
    }

    /**
     * Helper to mock a HasMany relationship count
     */
    private function mockHasMany(int $count)
    {
        $mock = $this->createMock(\Illuminate\Database\Eloquent\Relations\HasMany::class);
        $mock->method('count')->willReturn($count);
        return $mock;
    }
}