#!/usr/bin/env node
import * as cdk from 'aws-cdk-lib';
import { BackboneDokuServiceStack } from '../lib/backbone-doku-service-stack';

const envName = process.env.ENV_NAME!;

const app = new cdk.App();

new BackboneDokuServiceStack(app, `${envName}-BackboneDokuServiceStack`, {
	env: {
		account: process.env.CDK_DEFAULT_ACCOUNT,
		region: process.env.CDK_DEFAULT_REGION
	},
	vpcId: process.env.VPC_ID!,
	clusterName: process.env.CLUSTER_NAME!,
	listenerArn: process.env.LISTENER_ARN!,
	hostHeader: process.env.HOST_HEADER!,
	envBucketName: process.env.ENV_BUCKET_NAME!,
	envFileBucketName: process.env.ENV_FILE_BUCKET_NAME!
});
