package com.windshift.dcimport.rest;

import com.atlassian.jira.security.JiraAuthenticationContext;
import com.atlassian.jira.security.PermissionManager;
import com.atlassian.jira.security.Permissions;
import com.atlassian.jira.user.ApplicationUser;
import com.atlassian.plugin.spring.scanner.annotation.imports.ComponentImport;
import com.windshift.dcimport.workflow.WorkflowExtractor;
import jakarta.inject.Inject;
import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.PathParam;
import jakarta.ws.rs.Produces;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;

import java.util.Collections;
import java.util.Map;

/**
 * Returns a project's workflow scheme and each workflow's full graph, including
 * condition/validator descriptors.
 */
@Path("/project/{projectKey}/workflows")
public class WorkflowResource {

    private final WorkflowExtractor extractor;
    private final JiraAuthenticationContext authenticationContext;
    private final PermissionManager permissionManager;

    @Inject
    public WorkflowResource(WorkflowExtractor extractor,
                            @ComponentImport JiraAuthenticationContext authenticationContext,
                            @ComponentImport PermissionManager permissionManager) {
        this.extractor = extractor;
        this.authenticationContext = authenticationContext;
        this.permissionManager = permissionManager;
    }

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    public Response workflows(@PathParam("projectKey") String projectKey) {
        ApplicationUser user = authenticationContext.getLoggedInUser();
        if (user == null || !permissionManager.hasPermission(Permissions.ADMINISTER, user)) {
            return Response.status(Response.Status.FORBIDDEN).build();
        }
        return extractor.projectWorkflows(projectKey)
                .map(body -> Response.ok(body).build())
                .orElseGet(() -> Response.status(Response.Status.NOT_FOUND)
                        .entity(notFound(projectKey))
                        .build());
    }

    private Map<String, Object> notFound(String projectKey) {
        return Collections.singletonMap("error", "project not found: " + projectKey);
    }
}
