package com.ops.control.common;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.servlet.config.annotation.CorsRegistry;
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer;

@Configuration
@EnableConfigurationProperties(OpsProperties.class)
public class WebConfig implements WebMvcConfigurer {
    @Override
    public void addCorsMappings(CorsRegistry registry) {
        // JWT is in Authorization header; no cookie credentials needed.
        registry.addMapping("/api/**")
                .allowedOriginPatterns("*")
                .allowedMethods("*")
                .allowedHeaders("*");
    }
}
