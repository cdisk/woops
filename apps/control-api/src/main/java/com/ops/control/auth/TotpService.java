package com.ops.control.auth;

import dev.samstevens.totp.code.CodeGenerator;
import dev.samstevens.totp.code.DefaultCodeGenerator;
import dev.samstevens.totp.code.HashingAlgorithm;
import dev.samstevens.totp.exceptions.CodeGenerationException;
import dev.samstevens.totp.exceptions.QrGenerationException;
import dev.samstevens.totp.qr.QrData;
import dev.samstevens.totp.qr.QrGenerator;
import dev.samstevens.totp.qr.ZxingPngQrGenerator;
import dev.samstevens.totp.secret.DefaultSecretGenerator;
import dev.samstevens.totp.secret.SecretGenerator;
import dev.samstevens.totp.time.SystemTimeProvider;
import dev.samstevens.totp.time.TimeProvider;
import dev.samstevens.totp.util.Utils;
import org.springframework.stereotype.Service;

import java.util.OptionalLong;

@Service
public class TotpService {
    public static final String ISSUER = "Woops";
    /** Accept previous/next 30s window for modest clock skew (not unlimited reuse). */
    public static final int ALLOWED_SKEW_STEPS = 1;
    public static final int PERIOD_SECONDS = 30;

    private final SecretGenerator secretGenerator = new DefaultSecretGenerator(32);
    private final CodeGenerator codeGenerator = new DefaultCodeGenerator(HashingAlgorithm.SHA1, 6);
    private final TimeProvider timeProvider = new SystemTimeProvider();
    private final QrGenerator qrGenerator = new ZxingPngQrGenerator();

    public String generateSecret() {
        return secretGenerator.generate();
    }

    public boolean verify(String secret, String code) {
        return matchingStep(secret, code).isPresent();
    }

    /**
     * Returns the TOTP time-step (unix/30) that matched the code within
     * {@link #ALLOWED_SKEW_STEPS}, or empty if invalid.
     */
    public OptionalLong matchingStep(String secret, String code) {
        if (secret == null || secret.isBlank() || code == null) {
            return OptionalLong.empty();
        }
        String trimmed = code.trim().replace(" ", "");
        if (!trimmed.matches("\\d{6}")) {
            return OptionalLong.empty();
        }
        long current = timeProvider.getTime() / PERIOD_SECONDS;
        for (int delta = -ALLOWED_SKEW_STEPS; delta <= ALLOWED_SKEW_STEPS; delta++) {
            long step = current + delta;
            try {
                if (trimmed.equals(codeGenerator.generate(secret, step))) {
                    return OptionalLong.of(step);
                }
            } catch (CodeGenerationException e) {
                return OptionalLong.empty();
            }
        }
        return OptionalLong.empty();
    }

    public String otpauthUri(String username, String secret) {
        return qrData(username, secret).getUri();
    }

    /** PNG data URL for authenticator apps, or empty if QR generation fails. */
    public String qrDataUrl(String username, String secret) {
        try {
            byte[] image = qrGenerator.generate(qrData(username, secret));
            return Utils.getDataUriForImage(image, qrGenerator.getImageMimeType());
        } catch (QrGenerationException e) {
            return "";
        }
    }

    private static QrData qrData(String username, String secret) {
        String label = username == null || username.isBlank() ? "user" : username.trim();
        return new QrData.Builder()
                .label(label)
                .secret(secret)
                .issuer(ISSUER)
                .algorithm(HashingAlgorithm.SHA1)
                .digits(6)
                .period(PERIOD_SECONDS)
                .build();
    }
}
