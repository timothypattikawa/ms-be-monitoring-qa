import * as cdk from 'aws-cdk-lib';
import { Construct } from 'constructs';
import * as ecs from 'aws-cdk-lib/aws-ecs';
import * as elbv2 from 'aws-cdk-lib/aws-elasticloadbalancingv2';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as s3 from 'aws-cdk-lib/aws-s3';

export interface BackboneDokuServiceStackProps extends cdk.StackProps{
	vpcId: string;
	clusterName: string;
	listenerArn: string;
	hostHeader: string;
	envBucketName: string;
	envFileBucketName: string;
}

export class BackboneDokuServiceStack extends cdk.Stack {
	constructor(scope: Construct, id: string, props: BackboneDokuServiceStackProps) {
		super(scope, id, props);

		// Import VPC
		const vpc = ec2.Vpc.fromLookup(this, 'ImportedVPC', {
			vpcId: props.vpcId,
			isDefault: false
		});

		// Import ECS Cluster
		const cluster = ecs.Cluster.fromClusterAttributes(this, 'ImportedCluster', {
			clusterName: props.clusterName,
			vpc: vpc,
			securityGroups: []
		});

		// Import ALB Listener
		const listener = elbv2.ApplicationListener.fromLookup(
			this,
			'ImportedListener',
			{
				listenerArn: props.listenerArn
			}
		);

		// Task Execution Role
		const executionRole = new iam.Role(this, 'BackboneDokuExecutionRole', {
			assumedBy: new iam.ServicePrincipal('ecs-tasks.amazonaws.com'),
			managedPolicies: [
				iam.ManagedPolicy.fromAwsManagedPolicyName('service-role/AmazonECSTaskExecutionRolePolicy')
			],
			inlinePolicies: {
				S3Access: new iam.PolicyDocument({
					statements: [new iam.PolicyStatement({
						effect: iam.Effect.ALLOW,
						actions: ['s3:GetObject', 's3:ListBucket'],
						resources: [
							`arn:aws:s3:::${props.envBucketName}`,
							`arn:aws:s3:::${props.envBucketName}/*`
						]
					})]
				})
			}
		});

		// Role untuk ECS Task
		const taskRole = new iam.Role(this, 'BackboneDokuTaskRole', {
			assumedBy: new iam.ServicePrincipal('ecs-tasks.amazonaws.com')
		});

		// Task Definition
		const taskDef = new ecs.FargateTaskDefinition(this, 'BackboneDokuTaskDef', {
			memoryLimitMiB: 512,
			cpu: 256,
			taskRole,
			executionRole
		});

		taskDef.addContainer('BackboneDokuContainer', {
    // Kita tambahkan options 'exclude' di parameter kedua
    image: ecs.ContainerImage.fromAsset('../', {
        exclude: [
            'cdk.out',       // <--- PENTING: Jangan bungkus folder ini
            'node_modules',  // Opsional: Biar upload lebih cepat
            '.git',          // Opsional: Biar bersih
            'infra/cdk.out'  // Jaga-jaga kalau path-nya relative
        ]
    }),
    logging: ecs.LogDrivers.awsLogs({ streamPrefix: `BackboneDoku-${this.stackName}` }),
			portMappings: [{ containerPort: 3002 }],
			healthCheck: {
				command: ['CMD-SHELL', 'curl -f http://localhost:3002/v1.0/healthcheck/liveness || exit 1'],
				interval: cdk.Duration.seconds(10),
				timeout: cdk.Duration.seconds(5),
				retries: 3,
				startPeriod: cdk.Duration.seconds(30)
			},
			environmentFiles: [
				ecs.EnvironmentFile.fromBucket(
					s3.Bucket.fromBucketName(this, 'EnvBucket', props.envBucketName),
					props.envFileBucketName
				)
			]
		});

		// ECS Service dengan FARGATE_SPOT
		const service = new ecs.FargateService(this, 'BackboneDokuService', {
			cluster,
			taskDefinition: taskDef,
			desiredCount: 1,
			assignPublicIp: false,
			minHealthyPercent: 0,
			maxHealthyPercent: 200,
			capacityProviderStrategies: [
				{
					capacityProvider: 'FARGATE_SPOT',
					weight: 1
				}
			],
			vpcSubnets: {
				subnetType: ec2.SubnetType.PRIVATE_ISOLATED
			},
			circuitBreaker: {
				rollback: true
			}
		});

		// Buat Target Group baru untuk BackboneDoku service
		const targetGroup = new elbv2.ApplicationTargetGroup(this, 'BackboneDokuTG', {
			vpc,
			port: 3002,
			protocol: elbv2.ApplicationProtocol.HTTP,
			targetType: elbv2.TargetType.IP,
			healthCheck: {
				path: '/v1.0/healthcheck/liveness',
				healthyHttpCodes: '200'
			}
		});

		// Hubungkan ECS Service ke TG
		targetGroup.addTarget(service);

		// Tambah listener rule ke ALB → route `/BackboneDoku`
		listener.addAction('BackboneDokuRule', {
			priority: 90,
			conditions: [elbv2.ListenerCondition.hostHeaders([props.hostHeader])],
			action: elbv2.ListenerAction.forward([targetGroup])
		});
	}
}