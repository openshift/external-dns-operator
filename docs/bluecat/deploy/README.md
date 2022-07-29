# Copyright Note
This is a slightly modified copy of the original [terraform deploy](https://github.com/bluecatlabs/terraform_deploy.git).

# BlueCat Deployment via Terraform

## Purpose
Deploy BlueCat Address Manager, DNS/DHCP Server, Gateway to AWS instance using Terraform

## Pre-Requisites
1. Terraform must be installed on your workstation (version >= 1.1.7)
2. You must have admin access to an AWS account
3. A license file for the BAM and BDDS must be acquired

## Usage:

1. Initialize the Terraform state by running:  `terraform init`
   * This must be done from the root of the script repository
4. In the `files` folder:
   *  Update `bam_license.txt` with the license key and id you received from BlueCat
   *  Update `bdds_license.txt` with the license key and id you received from BlueCat
   *  Copy the private key of the keypair to be used into the folder
5. Run the below command to see the actions Terraform will take to deploy your infrastructure. Verify that these changes are correct before proceeding.
    ```sh
    terraform plan -var keypair=${MY_KEYPAIR} -var keypair_file=${MY_PRIVATE_KEY} -var access_key=${AWS_ACCESS_KEY_ID} -var access_secret=${AWS_SECRET_ACCESS_KEY} -var ami_gw=${ANY_AVAILABLE_UBUNTU_AMD64_AMI}
    ```
6. When ready to deploy, execute the below command, review the actions once again, and confirm:
    ```sh
    terraform apply -auto-approve -var keypair=${MY_KEYPAIR} -var keypair_file=${MY_PRIVATE_KEY} -var access_key=${AWS_ACCESS_KEY_ID} -var access_secret=${AWS_SECRET_ACCESS_KEY} -var ami_gw=${ANY_AVAILABLE_UBUNTU_AMD64_AMI}
    ```
7. When deployment is complete information about the environment created will be displayed.
    * The BAM console and Gateway console can be accessed using the public IP returned by `terraform apply`
    * Gateway will be deployed without custom workflows such as BlueCat Cloud Discovery & Visibility - please contact BlueCat for access.
8. When testing is complete execute the below command to clean up the AWS environment:
    ```sh
    terraform destroy -var keypair=${MY_KEYPAIR} -var keypair_file=${MY_PRIVATE_KEY} -var access_key=${AWS_ACCESS_KEY_ID} -var access_secret=${AWS_SECRET_ACCESS_KEY} -var ami_gw=${ANY_AVAILABLE_UBUNTU_AMD64_AMI}
    ```
