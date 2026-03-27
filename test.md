/multi

1. /sr Daily Todo. You will assist in accomplishing tasks on this list. Success Criteria is 100% completion within the day. Clearly high priority tasks are to be completed first. There is not enough context to complete each task off of this list. As we continue with this process, details will be added to openbrain to increase temporal context, spanning far beyond human recall with high fidelity. When you cross a task that requires additional context, you will request it. If there is no response within 5 minutes of a request, you are to send a series of dunst notifications: [urgency_level]

1st - "Context Request: Task [task name] requires additional context to proceed. Please provide the necessary information to continue." [normal]
2nd - "Reminder: Task [task name] is pending due to a lack of context. Please provide the necessary information to proceed." [normal] - change the notification to resemble our intent management system in ~/.claude . This will be an orange color
3rd - "Urgent: Task [task name] is still pending due to a lack of context. Immediate attention is required to provide the necessary information to proceed." [critical] - change the notification to resemble our intent management system in ~/.claude . This will be a red color and notification persists on screen until user removes it.

2. /sr The above request will ultimately run through a new agent that we will create called 'n0ko-assistant' - this agent will be focused on managing task lists, and the resources necessary to accomplish the tasks on them. This agent will be able to autonomously accomplish tasks on this list, and also be able to request additional context and resources as needed to accomplish these tasks. This agent will also be able to learn from my behaviors and preferences to better accomplish these tasks in a way that is aligned with my goals and preferences. Use today's list as a starting point, but also run this list against /spec --review --loop to create a more agent focused list that will allow you to be more autonomous in accomplishing these tasks, and future tasks. This list should be dynamic and change as you learn more about my preferences, behaviors, and the context around these tasks. You will also be able to add additional tasks that you identify as necessary to accomplish the goals of this list, and the overall goals of our work together.

   You will need to curate many of my writing samples to learn my writing style, and preferences for communication. This will allow you to better communicate with me, and also to create content that is more aligned with my style and preferences. This will be an ongoing process, and you will need to continuously update your understanding of my writing style and preferences as you learn more about me. These will also be stored in openbrain. There should be a style-map that emulates the content-engine with style types varying on communication medium (i.e. email, slack, dunst notifications, etc.) and also on the type of content (i.e. casual communication, formal communication, technical writing, etc.)

   Moving forward this list will be curated by an agent via sdk, with resources that allow claude to connect with (currently just google, working on azure) - here you will scrape content buckets/resources for neccessary information and will prioritize based on a gradual learning process of my behaviors, and preferences while conferring with openbrain for past context.

   As you require additional access you will request it, and keep these requests also in openbrain as a reminder of where you need to look for additional information and escalation if you do not have the access you need.

3. Today TODO: (this task should be actioned against from the spec generated from #1 -- consider this blocked until 1 is completed)
   Priority levels marked as: (only marked high priority items)
   [1] - High Priority
   [2] - Medium Priority
   [3] - Low Priority

- [ ] 1. Project specific agents for ComputeCommander
     spec-builder will need to be be run, against the --review --loop (these agents are for the purpose of iterating on the project via bug-fixes, feature-requests, security patches, version updates):
     1.1 cmdr-ux-agent (kdl and dashboard.go) - (read only)
     1.2 cmdr-openbrain-agent (read-only)
     1.3 cmdr-agent (agentic retrieval agent for cmdr)
     1.4 cmdr-jira - (read only)
     1.5 cmdr-bridge - (read only)
     1.6 cmdr-sercurity - (read only)
     1.7 cmdr-coder - (read and write)
     1.8 cmdr-reviewer (read-only)
- [ ] 2. Org-generator for Teradyne (this will not be a dry-run)
     2.1 Admin rights for: Atlassian/Azure: -- blocked by 3 (meeting this afternoon) - 1300
     - Jira
     - Confluence
       2.2 Azure rights:
       -@~/claude_teams.md
- [ ] 3. Outline Roadmap Agentic Workflows at Ecco -> [1] (meeting this afternoon) - 1300
- [ ] 4. Create a roadmap for Ecco's agentic workflows
     4.1 Identify key milestones and deliverables
     4.2 Define timelines and dependencies
     4.3 Assign responsibilities to team members
     -@~/claude_teams.md
- [ ] 5. "Morning Coffee Brief": [1]
- [ ] 6. google-cli
- [ ] 7. Create a google-cli agent for automating Google Workspace tasks
     7.1 Define the scope of tasks to be automated
     7.2 Develop the agent using appropriate APIs and libraries
     7.3 Test the agent's functionality
     7.4 Deploy the agent for use within the organization
     7.5 Responsd to iep meeting confirmation for tomorrow morning [1] -- VERY HIGH PRIORITY
- [ ] 8. check cron for lmr-script (ensure it's running, convert to a service to persist restarts) [1]
- [ ] 9. rayne development:
     9.1 dual ship to both personal github and ecco-github
     9.1.1 Update github profile to exclude this project
     9.1.2 Update rayne frontend to also not include this project
     9.1.3 Update rayne frontend rag pipeline to reference project
     9.2 Optimize existing functionalities
     9.3 Conduct thorough testing
     9.4 Prepare documentation and training materials
     9.5 Plan and execute the deployment of the updated Rayne system
- [ ] 10. Terraform.ai project with Chris (meeting this afternoon) - 1400
- [ ] 11. Ensure that I can jump around between github projects -- prerequisite for 9
      11.1Tools Impacted: - 'gh' - 'github workflows'
- [ ] 12. Refresher on my argocd workflows -- prerequisite for 10
      review ~/.zsh/tooling/argo.zsh
      optimize ~/.zsh/tooling/argo.zsh

# Outcomes

1. Management of the current daily task list
2. A spec that is executed against that creates a 'n0ko-assistant' agent that is focused on managing task lists, and the resources necessary to accomplish the tasks on them. This agent will be able to autonomously accomplish tasks on this list, and also be able to request additional context and resources as needed to accomplish these tasks. This agent will also be able to learn from my behaviors and preferences to better accomplish these tasks in a way that is aligned with my goals and preferences.
3. An established communication (bridge) between 'n0ko-assistant' and 'openbrain' to allow for seamless access to context and resources stored in openbrain, as well as the ability to update openbrain with new information and context as it is acquired through the process of accomplishing tasks on this list. This will allow for a more efficient and effective way of managing and accessing the information and resources necessary to accomplish these tasks, as well as a way to keep track of the progress and outcomes of these tasks in a centralized location.
4. Execution against the spec generated for this list
