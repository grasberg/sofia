"""
Pytest configuration and fixtures for database testing.
"""
import os
import pytest
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker
from app.database import Base, get_db, engine as default_engine, SessionLocal as default_SessionLocal
from app.models import (
    Affiliate, AffiliateLink, AffiliateClick,
    AffiliateConversion, AffiliatePayout,
    Lead, Order, Product
)

# Use in-memory SQLite for tests
TEST_DATABASE_URL = "sqlite:///:memory:"


@pytest.fixture(scope="session", autouse=True)
def set_test_env():
    """Set environment variables for testing."""
    os.environ["DATABASE_URL"] = TEST_DATABASE_URL
    os.environ["TESTING"] = "true"
    yield
    # Clean up
    if "DATABASE_URL" in os.environ:
        del os.environ["DATABASE_URL"]
    if "TESTING" in os.environ:
        del os.environ["TESTING"]


@pytest.fixture(scope="session")
def test_engine():
    """Create a test database engine."""
    engine = create_engine(
        TEST_DATABASE_URL,
        connect_args={"check_same_thread": False}
    )
    yield engine
    engine.dispose()


@pytest.fixture(scope="session")
def tables(test_engine):
    """Create all tables once per test session."""
    Base.metadata.create_all(bind=test_engine)
    yield
    Base.metadata.drop_all(bind=test_engine)


@pytest.fixture(scope="session", autouse=True)
def override_database(test_engine, tables):
    """Monkey-patch the database engine and SessionLocal for testing."""
    from app import database
    
    # Save original values
    original_engine = database.engine
    original_SessionLocal = database.SessionLocal
    
    # Replace with test versions
    database.engine = test_engine
    database.SessionLocal = sessionmaker(
        autocommit=False, 
        autoflush=False, 
        bind=test_engine
    )
    
    yield
    
    # Restore original values
    database.engine = original_engine
    database.SessionLocal = original_SessionLocal


@pytest.fixture
def db_session(test_engine, tables):
    """Provide a database session for each test."""
    connection = test_engine.connect()
    transaction = connection.begin()
    Session = sessionmaker(bind=connection)
    session = Session()
    
    try:
        yield session
    finally:
        session.close()
        transaction.rollback()
        connection.close()


@pytest.fixture
def client(db_session):
    """Create a test client that uses the test database."""
    # Override the get_db dependency to use our test session
    from fastapi.testclient import TestClient
    from app.main import app
    
    def override_get_db():
        try:
            yield db_session
        finally:
            pass
    
    app.dependency_overrides[get_db] = override_get_db
    client = TestClient(app)
    yield client
    app.dependency_overrides.clear()


@pytest.fixture
def sample_affiliate(db_session):
    """Create a sample affiliate for testing."""
    affiliate = Affiliate(
        name="Test Affiliate",
        email="affiliate@test.com",
        commission_rate=15.0,
        status="active"
    )
    db_session.add(affiliate)
    db_session.commit()
    db_session.refresh(affiliate)
    return affiliate


@pytest.fixture
def sample_product(db_session):
    """Create a sample product for testing."""
    product = Product(
        name="Test Product",
        description="A test product",
        price=49.99,
        stripe_product_id="prod_test123",
        stripe_price_id="price_test123",
        is_active=True
    )
    db_session.add(product)
    db_session.commit()
    db_session.refresh(product)
    return product