package com.windshift.dcimport.workflow;

import com.atlassian.jira.issue.status.Status;
import com.atlassian.jira.project.Project;
import com.atlassian.jira.project.ProjectManager;
import com.atlassian.jira.workflow.JiraWorkflow;
import com.atlassian.jira.workflow.WorkflowManager;
import com.atlassian.jira.workflow.WorkflowSchemeManager;
import com.atlassian.plugin.spring.scanner.annotation.imports.ComponentImport;
import com.opensymphony.workflow.loader.ActionDescriptor;
import com.opensymphony.workflow.loader.ConditionDescriptor;
import com.opensymphony.workflow.loader.ConditionalResultDescriptor;
import com.opensymphony.workflow.loader.ConditionsDescriptor;
import com.opensymphony.workflow.loader.FunctionDescriptor;
import com.opensymphony.workflow.loader.RestrictionDescriptor;
import com.opensymphony.workflow.loader.ResultDescriptor;
import com.opensymphony.workflow.loader.StepDescriptor;
import com.opensymphony.workflow.loader.ValidatorDescriptor;
import com.opensymphony.workflow.loader.WorkflowDescriptor;
import jakarta.inject.Inject;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;

/**
 * Reads a project's configured workflow graph directly from Jira's workflow
 * managers, including the condition/validator/post-function descriptors that the
 * public REST API does not expose.
 */
@Component
public class WorkflowExtractor {

    private final ProjectManager projectManager;
    private final WorkflowManager workflowManager;
    private final WorkflowSchemeManager workflowSchemeManager;

    @Inject
    public WorkflowExtractor(@ComponentImport ProjectManager projectManager,
                             @ComponentImport WorkflowManager workflowManager,
                             @ComponentImport WorkflowSchemeManager workflowSchemeManager) {
        this.projectManager = projectManager;
        this.workflowManager = workflowManager;
        this.workflowSchemeManager = workflowSchemeManager;
    }

    public Optional<Map<String, Object>> projectWorkflows(String projectKey) {
        Project project = projectManager.getProjectObjByKey(projectKey);
        if (project == null) {
            return Optional.empty();
        }
        Map<String, String> workflowMap = workflowSchemeManager.getWorkflowMap(project);
        Set<String> names = new LinkedHashSet<>(workflowMap.values());

        // Jira keys the default workflow mapping with null; JSON needs a real key.
        Map<String, String> issueTypeWorkflowIds = new LinkedHashMap<>();
        String defaultWorkflow = null;
        for (Map.Entry<String, String> entry : workflowMap.entrySet()) {
            if (entry.getKey() == null) {
                defaultWorkflow = entry.getValue();
            } else {
                issueTypeWorkflowIds.put(entry.getKey(), entry.getValue());
            }
        }

        List<Map<String, Object>> workflows = new ArrayList<>();
        for (String name : names) {
            if (name == null || name.isEmpty()) {
                continue;
            }
            JiraWorkflow workflow = workflowManager.getWorkflow(name);
            if (workflow != null) {
                workflows.add(workflow(workflow));
            }
        }

        Map<String, Object> projectSummary = new LinkedHashMap<>();
        projectSummary.put("id", project.getId() == null ? null : project.getId().toString());
        projectSummary.put("key", project.getKey());
        projectSummary.put("name", project.getName());

        Map<String, Object> result = new LinkedHashMap<>();
        result.put("project", projectSummary);
        result.put("defaultWorkflow", defaultWorkflow);
        result.put("issueTypeWorkflowIds", issueTypeWorkflowIds);
        result.put("workflows", workflows);
        return Optional.of(result);
    }

    private Map<String, Object> workflow(JiraWorkflow workflow) {
        WorkflowDescriptor descriptor = workflow.getDescriptor();

        List<Map<String, Object>> statuses = new ArrayList<>();
        for (Status status : workflow.getLinkedStatusObjects()) {
            Map<String, Object> summary = new LinkedHashMap<>();
            summary.put("id", status.getId());
            summary.put("name", status.getName());
            statuses.add(summary);
        }

        Map<Integer, String> stepStatusIds = new HashMap<>();
        for (Object raw : descriptor.getSteps()) {
            StepDescriptor step = (StepDescriptor) raw;
            String statusId = workflow.getLinkedStatusId(step);
            if (statusId != null) {
                stepStatusIds.put(step.getId(), statusId);
            }
        }

        List<Map<String, Object>> transitions = new ArrayList<>();
        Set<Integer> seenActionIds = new HashSet<>();
        for (Object raw : descriptor.getInitialActions()) {
            ActionDescriptor action = (ActionDescriptor) raw;
            if (seenActionIds.add(action.getId())) {
                transitions.add(action(action, "INITIAL", null, stepStatusIds));
            }
        }
        // Jira's simplified workflow keeps its global transitions here.
        for (Object raw : descriptor.getGlobalActions()) {
            ActionDescriptor action = (ActionDescriptor) raw;
            if (seenActionIds.add(action.getId())) {
                transitions.add(action(action, "GLOBAL", null, stepStatusIds));
            }
        }
        for (Object raw : descriptor.getCommonActions().values()) {
            ActionDescriptor action = (ActionDescriptor) raw;
            if (seenActionIds.add(action.getId())) {
                transitions.add(action(action, "GLOBAL", null, stepStatusIds));
            }
        }
        for (Object raw : descriptor.getSteps()) {
            StepDescriptor step = (StepDescriptor) raw;
            String fromStatusId = stepStatusIds.get(step.getId());
            for (Object actionRaw : step.getActions()) {
                ActionDescriptor action = (ActionDescriptor) actionRaw;
                if (seenActionIds.add(action.getId())) {
                    transitions.add(action(action, "DIRECTED", fromStatusId, stepStatusIds));
                }
            }
        }

        Map<String, Object> out = new LinkedHashMap<>();
        out.put("id", workflow.getName());
        out.put("name", workflow.getName());
        out.put("statuses", statuses);
        out.put("transitions", transitions);
        return out;
    }

    private Map<String, Object> action(ActionDescriptor action, String type, String fromStatusId,
                                       Map<Integer, String> stepStatusIds) {
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("id", String.valueOf(action.getId()));
        out.put("name", action.getName());
        out.put("type", type);
        if (fromStatusId != null) {
            out.put("fromStatusIds", Collections.singletonList(fromStatusId));
        }
        ResultDescriptor result = action.getUnconditionalResult();
        if (result != null) {
            String toStatusId = stepStatusIds.get(result.getStep());
            if (toStatusId != null) {
                out.put("toStatusId", toStatusId);
            }
            out.put("postFunctions", descriptors(result.getPostFunctions()));
        }
        out.put("conditions", conditions(action));
        out.put("validators", descriptors(action.getValidators()));
        return out;
    }

    private List<Map<String, Object>> conditions(ActionDescriptor action) {
        List<Map<String, Object>> out = new ArrayList<>();
        // Jira stores transition conditions in <restrict-to><conditions>.
        RestrictionDescriptor restriction = action.getRestriction();
        if (restriction != null && restriction.getConditionsDescriptor() != null) {
            collectConditions(restriction.getConditionsDescriptor(), out);
        }
        // Some workflows also gate on conditional results; keep those too.
        for (Object raw : action.getConditionalResults()) {
            ConditionalResultDescriptor conditional = (ConditionalResultDescriptor) raw;
            for (Object groupRaw : conditional.getConditions()) {
                collectConditions((ConditionsDescriptor) groupRaw, out);
            }
        }
        return out;
    }

    private void collectConditions(ConditionsDescriptor group, List<Map<String, Object>> out) {
        for (Object raw : group.getConditions()) {
            if (raw instanceof ConditionsDescriptor) {
                collectConditions((ConditionsDescriptor) raw, out);
            } else if (raw instanceof ConditionDescriptor) {
                ConditionDescriptor condition = (ConditionDescriptor) raw;
                out.add(descriptor(condition.getType(), condition.getArgs()));
            }
        }
    }

    private List<Map<String, Object>> descriptors(List<?> descriptors) {
        List<Map<String, Object>> out = new ArrayList<>();
        for (Object raw : descriptors) {
            if (raw instanceof ValidatorDescriptor) {
                ValidatorDescriptor validator = (ValidatorDescriptor) raw;
                out.add(descriptor(validator.getType(), validator.getArgs()));
            } else if (raw instanceof FunctionDescriptor) {
                FunctionDescriptor function = (FunctionDescriptor) raw;
                out.add(descriptor(function.getType(), function.getArgs()));
            }
        }
        return out;
    }

    private Map<String, Object> descriptor(String type, Map<?, ?> args) {
        String className = args == null ? null : asString(args.get("class.name"));
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("type", type);
        out.put("class", className);
        out.put("kind", kind(className));
        out.put("args", args == null ? Collections.emptyMap() : args);
        return out;
    }

    private String asString(Object value) {
        return value == null ? null : String.valueOf(value);
    }

    /** Normalized view of the built-ins the importer can map faithfully. */
    private String kind(String className) {
        if (className == null) {
            return "unknown";
        }
        switch (className) {
            case "com.atlassian.jira.workflow.condition.UserIsInGroupCondition":
                return "user_in_group";
            case "com.atlassian.jira.workflow.condition.UserIsInProjectRoleCondition":
                return "user_in_role";
            case "com.atlassian.jira.workflow.condition.AllowOnlyAssignee":
                return "only_assignee";
            case "com.atlassian.jira.workflow.condition.PermissionCondition":
            case "com.atlassian.jira.workflow.validator.PermissionValidator":
                return "permission";
            case "com.atlassian.jira.workflow.condition.SubTaskBlockingCondition":
                return "subtask_blocking";
            default:
                return "unknown";
        }
    }
}
