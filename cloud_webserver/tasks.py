from celery_app import celery
import time
import logging

logger = logging.getLogger(__name__)

@celery.task
def task():
    logger.info('start')
    time.sleep(10)
    logger.info('end')
