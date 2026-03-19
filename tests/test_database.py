"""
Basic database tests using SQLite in-memory database.
"""
import pytest
from sqlalchemy.exc import IntegrityError
from app.models import Affiliate, Product


def test_create_tables(db_session):
    """Test that tables can be created and are empty initially."""
    # Count initial rows
    affiliate_count = db_session.query(Affiliate).count()
    product_count = db_session.query(Product).count()
    
    assert affiliate_count == 0
    assert product_count == 0


def test_create_affiliate(db_session):
    """Test creating an affiliate record."""
    affiliate = Affiliate(
        name="Test Affiliate",
        email="test@example.com",
        commission_rate=10.0,
        status="active"
    )
    db_session.add(affiliate)
    db_session.commit()
    
    # Verify the record was created
    retrieved = db_session.query(Affiliate).filter_by(email="test@example.com").first()
    assert retrieved is not None
    assert retrieved.name == "Test Affiliate"
    assert retrieved.commission_rate == 10.0
    assert retrieved.status == "active"


def test_create_product(db_session):
    """Test creating a product record."""
    product = Product(
        name="Test Product",
        description="A test product",
        price=29.99,
        stripe_product_id="prod_test",
        stripe_price_id="price_test",
        is_active=True
    )
    db_session.add(product)
    db_session.commit()
    
    retrieved = db_session.query(Product).filter_by(name="Test Product").first()
    assert retrieved is not None
    assert retrieved.price == 29.99
    assert retrieved.is_active == True


def test_affiliate_unique_email(db_session):
    """Test that email must be unique for affiliates."""
    affiliate1 = Affiliate(
        name="Affiliate 1",
        email="same@example.com",
        commission_rate=10.0
    )
    affiliate2 = Affiliate(
        name="Affiliate 2",
        email="same@example.com",  # Same email
        commission_rate=15.0
    )
    
    db_session.add(affiliate1)
    db_session.commit()
    
    db_session.add(affiliate2)
    with pytest.raises(IntegrityError):
        db_session.commit()
    
    db_session.rollback()