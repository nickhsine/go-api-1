ALTER TABLE `pay_by_prime_donations` MODIFY `status` enum('paying', 'paid', 'fail');
ALTER TABLE `pay_by_card_token_donations` MODIFY `status` enum('paying', 'paid', 'fail');
