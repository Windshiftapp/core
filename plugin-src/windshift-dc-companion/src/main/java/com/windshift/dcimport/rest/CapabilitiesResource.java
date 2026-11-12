package com.windshift.dcimport.rest;

import com.atlassian.jira.security.JiraAuthenticationContext;
import com.atlassian.jira.security.PermissionManager;
import com.atlassian.jira.security.Permissions;
import com.atlassian.jira.user.ApplicationUser;
import com.atlassian.plugin.spring.scanner.annotation.imports.ComponentImport;
import jakarta.inject.Inject;
import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.Produces;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;

import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Discovery endpoint. The importer probes this and falls back to the public
 * Jira REST API when it is absent.
 */
@Path("/capabilities")
public class CapabilitiesResource {

    private static final String PLUGIN_VERSION = "1.0.0";

    private final JiraAuthenticationContext authenticationContext;
    private final PermissionManager permissionManager;

    @Inject
    public CapabilitiesResource(@ComponentImport JiraAuthenticationContext authenticationContext,
                                @ComponentImport PermissionManager permissionManager) {
        this.authenticationContext = authenticationContext;
        this.permissionManager = permissionManager;
    }

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    public Response capabilities() {
        ApplicationUser user = authenticationContext.getLoggedInUser();
        if (user == null || !permissionManager.hasPermission(Permissions.ADMINISTER, user)) {
            return Response.status(Response.Status.FORBIDDEN).build();
        }
        Map<String, Object> body = new LinkedHashMap<>();
        body.put("pluginVersion", PLUGIN_VERSION);
        body.put("deploymentType", "datacenter");
        body.put("capabilities", Arrays.asList("workflows", "screens"));
        return Response.ok(body).build();
    }
}
